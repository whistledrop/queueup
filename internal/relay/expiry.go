package relay

import (
	"context"
	"time"

	"queueup/internal/store"
)

// Jobs used to wait forever for a PC that never came back.
//
// Found on 12 September 2026: a job created on 31 August was still pending
// twelve days later, because the PC it belonged to went offline and stayed
// offline. Two things wrong with that. The moment such a PC is switched on it
// launches Rust and joins a server somebody asked for a fortnight ago, which is
// alarming rather than helpful. And since one PC runs one job, that ancient job
// silently blocks every new one.
//
// The brief always asked for this: keep trying to hand the job over "for a
// configurable window". The trying was built; the window was not.
const DefaultJobExpiry = 6 * time.Hour

// jobHasExpired decides whether one job has waited long enough.
//
// Two guards matter more than the clock. A job on a PC that is CONNECTED is
// never stale, however quiet: a job waiting for a wipe restart can sit for
// hours without a single update, and killing that would break the product's
// main feature. And a job that is already finished is left alone.
func jobHasExpired(j store.Job, deviceOnline bool, now time.Time, window time.Duration) bool {
	if !j.Active() || deviceOnline || window <= 0 {
		return false
	}
	return now.Sub(j.UpdatedAt) >= window
}

// RunJobExpiry closes joins whose PC never came back, until ctx ends.
func (s *Server) RunJobExpiry(ctx context.Context, every, window time.Duration) {
	if every <= 0 {
		every = 5 * time.Minute
	}
	if window <= 0 {
		window = DefaultJobExpiry
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.sweepExpiredJobs(window)
		}
	}
}

func (s *Server) sweepExpiredJobs(window time.Duration) {
	jobs, err := s.st.ActiveJobs()
	if err != nil {
		s.log.Error("listing jobs to expire", "err", err)
		return
	}
	now := time.Now()
	for _, j := range jobs {
		if !jobHasExpired(j, s.hub.Online(j.DeviceID), now, window) {
			continue
		}
		const msg = "Your PC didn't come back in time, so this join was cancelled. Start it again whenever the PC is on."
		s.log.Info("closing a join whose PC never came back",
			"job", j.ID, "device", j.DeviceID, "idle", now.Sub(j.UpdatedAt).Round(time.Minute))
		if err := s.st.FinishJob(j.ID, "failed", "expired", msg); err != nil {
			s.log.Error("expiring a job", "job", j.ID, "err", err)
			continue
		}
		if events, err := s.st.Events(j.ID, 0); err == nil && len(events) > 0 {
			s.hub.Publish(events[len(events)-1])
		}
	}
}
