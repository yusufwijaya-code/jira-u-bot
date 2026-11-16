package chat_group

import (
	"context"
	"errors"
	"log"
	"time"
	"unicode/utf8"
)

const (
	sweepEvery = time.Hour
	sweepLimit = 50
	maxErrLen  = 255
)

// Adder is the Groups API call (Client.AddMember).
type Adder interface {
	AddMember(ctx context.Context, email string) error
}

type Service struct {
	adder Adder
	store Store
	pause time.Duration // between sweep calls; 0 in tests
}

func NewService(adder Adder, store Store) *Service {
	return &Service{adder: adder, store: store, pause: 200 * time.Millisecond}
}

// shorten keeps msg within maxErrLen characters (VARCHAR(255) counts runes).
func shorten(msg string) string {
	if utf8.RuneCountInString(msg) <= maxErrLen {
		return msg
	}
	return string([]rune(msg)[:maxErrLen])
}

// setupError: the operator's token or group is broken, not the user.
func setupError(err error) bool {
	return errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrGroupConfig)
}

func logSetupError(err error) {
	if errors.Is(err, ErrUnauthorized) {
		log.Printf("chat_group: ERROR operator Groups token rejected (401) — run --setup-groups-oauth again and restart")
		return
	}
	log.Printf("chat_group: ERROR %v — check NUDGE_GROUP_EMAIL and that the operator owns/manages the group", err)
}

// join adds one email and records the outcome; setup errors are returned
// unrecorded so the user's dashboard doesn't blame them.
func (s *Service) join(ctx context.Context, email string) error {
	err := s.adder.AddMember(ctx, email)
	switch {
	case err == nil:
		if mErr := s.store.MarkAdded(ctx, email); mErr != nil {
			log.Printf("chat_group: marking %s added failed: %v", email, mErr)
		}
		return nil
	case setupError(err):
		return err
	}
	log.Printf("chat_group: adding %s failed: %v", email, err)
	if mErr := s.store.MarkFailed(ctx, email, shorten(err.Error())); mErr != nil {
		log.Printf("chat_group: recording the failure of %s failed: %v", email, mErr)
	}
	return err
}

// EnsureMember adds email to the group when it still needs to be (login hook).
func (s *Service) EnsureMember(ctx context.Context, email string) {
	need, err := s.store.NeedsJoin(ctx, email)
	if err != nil {
		log.Printf("chat_group: NeedsJoin(%s) failed: %v", email, err)
		return
	}
	if !need {
		return
	}
	if err := s.join(ctx, email); setupError(err) {
		logSetupError(err)
	}
}

// Sweep adds up to sweepLimit pending users and returns how many joined;
// it stops at the first setup error (401 or group not accessible).
func (s *Service) Sweep(ctx context.Context) int {
	emails, err := s.store.Pending(ctx, sweepLimit)
	if err != nil {
		log.Printf("chat_group: listing pending users failed: %v", err)
		return 0
	}
	joined := 0
	for _, e := range emails {
		err := s.join(ctx, e)
		if setupError(err) {
			logSetupError(err)
			break
		}
		if err == nil {
			joined++
		}
		select {
		case <-ctx.Done():
			return joined
		case <-time.After(s.pause):
		}
	}
	return joined
}

// Run sweeps now and then every hour until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		if n := s.Sweep(ctx); n > 0 {
			log.Printf("chat_group: %d user(s) added to the Nudge group", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
