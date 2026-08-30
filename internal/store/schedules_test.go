package store_test

import (
	"testing"
	"time"

	"queueup/internal/store"
)

// The Spain/UK case, in miniature: a schedule set from a phone in one timezone
// must be stored as UTC and mean the same instant everywhere.
func TestScheduleTimesAreStoredAsUTC(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)

	madrid := time.FixedZone("Europe/Madrid (summer)", 2*60*60)
	fireLocal := time.Date(2026, 9, 3, 20, 0, 0, 0, madrid) // 20:00 in Spain

	sc, err := s.CreateSchedule(store.NewSchedule{
		AccountID: acct.ID, DeviceID: d.ID,
		ServerAddr: "1.2.3.4:28015", FireAt: fireLocal,
	})
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}

	got, err := s.ScheduleByID(sc.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Same instant...
	if !got.FireAt.Equal(fireLocal) {
		t.Fatalf("stored %v does not equal the instant that was set (%v)", got.FireAt, fireLocal)
	}
	// ...expressed in UTC: 20:00 Madrid summer time is 18:00 UTC, which is
	// 19:00 on the PC in the UK.
	if got.FireAt.Location() != time.UTC || got.FireAt.Hour() != 18 {
		t.Fatalf("stored as %v, want 18:00 UTC", got.FireAt)
	}
}

func TestScheduleRefusesThePastAndTheFarFuture(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)
	base := store.NewSchedule{AccountID: acct.ID, DeviceID: d.ID, ServerAddr: "1.2.3.4:28015"}

	past := base
	past.FireAt = time.Now().Add(-time.Hour)
	if _, err := s.CreateSchedule(past); err == nil {
		t.Error("a time in the past was accepted")
	}

	far := base
	far.FireAt = time.Now().Add(60 * 24 * time.Hour)
	if _, err := s.CreateSchedule(far); err == nil {
		t.Error("a time two months away was accepted")
	}
}

// A schedule must fire exactly once, no matter how often the scheduler asks.
func TestDueSchedulesFireOnce(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)

	clock := time.Date(2026, 9, 3, 18, 0, 0, 0, time.UTC)
	s.SetClockForTest(func() time.Time { return clock })

	sc, err := s.CreateSchedule(store.NewSchedule{
		AccountID: acct.ID, DeviceID: d.ID,
		ServerAddr: "1.2.3.4:28015", FireAt: clock.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Not yet.
	if due, _ := s.DueSchedules(); len(due) != 0 {
		t.Fatalf("fired %d schedules before their time", len(due))
	}

	clock = clock.Add(11 * time.Minute)
	due, err := s.DueSchedules()
	if err != nil || len(due) != 1 || due[0].ID != sc.ID {
		t.Fatalf("DueSchedules = %v, %v; want exactly the one schedule", due, err)
	}

	// Never again.
	for i := 0; i < 3; i++ {
		if due, _ := s.DueSchedules(); len(due) != 0 {
			t.Fatalf("the same schedule fired twice")
		}
	}
}

func TestCancelScheduleIsForItsOwnerAndOnlyBeforeItFires(t *testing.T) {
	s := newStore(t)
	acct, d := pairedDevice(t, s)
	sc, err := s.CreateSchedule(store.NewSchedule{
		AccountID: acct.ID, DeviceID: d.ID,
		ServerAddr: "1.2.3.4:28015", FireAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	other, _, _ := s.CreateAccount("other@example.com")
	if err := s.CancelSchedule(other.ID, sc.ID); err == nil {
		t.Fatal("another account cancelled this schedule")
	}
	if err := s.CancelSchedule(acct.ID, sc.ID); err != nil {
		t.Fatalf("the owner couldn't cancel: %v", err)
	}
	if err := s.CancelSchedule(acct.ID, sc.ID); err == nil {
		t.Fatal("a cancelled schedule was cancelled again")
	}
	got, _ := s.ScheduleByID(sc.ID)
	if got.State != "cancelled" {
		t.Fatalf("state = %s, want cancelled", got.State)
	}
}
