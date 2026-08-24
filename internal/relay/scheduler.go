package relay

import (
	"context"
	"fmt"
	"time"

	"queueup/internal/notify"
	"queueup/internal/store"
)

// RunScheduler fires planned joins when their time comes. Schedules live on the
// relay and fire on the relay's clock: the phone can be asleep in Spain and the
// PC idle in the UK, and the join still starts on time.
func (s *Server) RunScheduler(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = 5 * time.Second
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			due, err := s.st.DueSchedules()
			if err != nil {
				s.log.Error("reading due schedules", "err", err)
				continue
			}
			for _, sc := range due {
				s.fireSchedule(sc)
			}
		}
	}
}

// fireSchedule turns one due schedule into a running job.
func (s *Server) fireSchedule(sc store.Schedule) {
	s.log.Info("schedule firing", "schedule", sc.ID, "server", sc.ServerName, "device", sc.DeviceID)

	fail := func(note string) {
		_ = s.st.ResolveSchedule(sc.ID, "failed", note, "")
		s.notifier.Send(sc.AccountID, notify.Message{
			Title: "Scheduled join couldn't start",
			Body:  note,
			Tag:   "schedule-" + sc.ID,
			URL:   "/",
		})
	}

	device, err := s.st.DeviceByID(sc.DeviceID)
	if err != nil || device.Revoked() {
		fail("The PC this join was scheduled for is no longer linked to your account.")
		return
	}

	if s.cfg.BillingEnabled {
		if sub, err := s.st.SubscriptionFor(sc.AccountID); err != nil || !sub.Active() {
			fail("Your subscription ended before this join fired. Resubscribe and schedule it again.")
			return
		}
	}

	// One PC, one job: a scheduled join yields to whatever is already running,
	// and says so, rather than tearing down a queue place already earned.
	if existing, err := s.st.ActiveJobForDevice(sc.DeviceID); err == nil {
		fail(fmt.Sprintf("Your PC was already busy with another join (%s).",
			existing.ServerName))
		return
	}

	addr, name, queryAddr := sc.ServerAddr, sc.ServerName, ""
	if sc.ServerID != "" && s.cfg.Servers != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		sv, err := s.cfg.Servers.ByID(ctx, sc.ServerID)
		cancel()
		if err == nil && sv.Address != "" {
			addr, name, queryAddr = sv.Address, sv.Name, sv.QueryAddress
		}
	}
	if addr == "" {
		fail("We couldn't work out that server's address when the time came.")
		return
	}

	j, err := s.st.CreateJob(store.NewJob{
		AccountID:       sc.AccountID,
		DeviceID:        sc.DeviceID,
		ServerAddr:      addr,
		ServerName:      name,
		ServerID:        sc.ServerID,
		QueryAddr:       queryAddr,
		WaitForServerUp: sc.WaitForServerUp,
	})
	if err != nil {
		s.log.Error("creating job from schedule", "err", err)
		fail("Something went wrong starting the join. The timeline has details.")
		return
	}
	_ = s.st.ResolveSchedule(sc.ID, "fired", "Started on time.", j.ID)

	online := s.hub.Online(sc.DeviceID)
	s.dispatch(j, false, true)

	if online {
		s.notifier.Send(sc.AccountID, notify.Message{
			Title: "Scheduled join started",
			Body:  fmt.Sprintf("Your PC is joining %s now.", name),
			Tag:   "job-" + j.ID,
			URL:   "/jobs/" + j.ID,
		})
	} else {
		// The job is saved and will start the moment the PC returns; the user
		// needs to know now, while there may still be time to fix the PC.
		s.notifier.Send(sc.AccountID, notify.Message{
			Title: "Your PC is offline",
			Body:  fmt.Sprintf("The scheduled join for %s is ready, but your PC isn't connected. It will start the moment the PC comes back.", name),
			Tag:   "job-" + j.ID,
			URL:   "/jobs/" + j.ID,
		})
	}
}
