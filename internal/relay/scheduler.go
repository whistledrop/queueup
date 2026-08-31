package relay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"queueup/internal/protocol"
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
	}

	device, err := s.st.DeviceByID(sc.DeviceID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail("The PC this join was scheduled for is no longer linked to your account.")
		return
	case err == nil && device.Revoked():
		fail("The PC this join was scheduled for has been unlinked from your account.")
		return
	case err != nil:
		// Could not tell. Carry on: creating the job will fail properly if the
		// PC really is gone, and a revoked PC cannot connect anyway.
		s.log.Error("reading the device for a due schedule, continuing",
			"schedule", sc.ID, "err", err)
	}

	if s.cfg.BillingEnabled {
		sub, err := s.st.SubscriptionFor(sc.AccountID)
		if subscriptionBlocks(sub, err) {
			fail("Your subscription ended before this join fired. Resubscribe and schedule it again.")
			return
		}
		if err != nil {
			s.log.Error("reading the subscription for a due schedule, letting it through",
				"schedule", sc.ID, "err", err)
		}
	}

	// One PC, one job. If something is still running when a planned join comes
	// due, the planned one wins: somebody deliberately set it, usually for the
	// wipe, and nobody deliberately arranged for the other join to still be
	// going. Yielding here is how a wipe gets silently missed.
	if existing, err := s.st.ActiveJobForDevice(sc.DeviceID); err == nil {
		name := sc.ServerName
		if name == "" {
			name = sc.ServerAddr
		}
		s.log.Info("a scheduled join is taking over from a running one",
			"schedule", sc.ID, "stopping", existing.ID)
		s.note(existing.ID, existing.State,
			fmt.Sprintf("Stopped: your scheduled join for %s is starting now.", name))
		if err := s.st.FinishJob(existing.ID, "done", "superseded",
			fmt.Sprintf("Your scheduled join for %s took over.", name)); err != nil {
			s.log.Error("standing down the running job", "err", err)
		}
		// Tell the PC to stop, so Rust is not left queueing for the old server
		// while the new job launches.
		_ = s.hub.SendTo(sc.DeviceID, protocol.TypeCancel, protocol.Cancel{
			JobID:  existing.ID,
			Reason: fmt.Sprintf("Your scheduled join for %s is starting now.", name),
		})
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

	s.dispatch(j, false, true)
}

// subscriptionBlocks reports whether we POSITIVELY know this account may not
// join. An unreadable answer is not a no.
//
// By the time a schedule fires it has already been claimed, so it cannot fire
// again: there is no "try later", only proceed or cancel. Letting an unreadable
// answer through costs at most one join that should not have run. Cancelling
// costs a paying customer their wipe, and tells them their subscription ended
// when it did not. The asymmetry is the whole argument.
func subscriptionBlocks(sub store.Subscription, err error) bool {
	if err != nil {
		return false
	}
	return !sub.Active()
}
