package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/calculator/app/calc"
)

// EventCalcUpdated is "calc.updated"
type EventCalcUpdated struct {
	InstanceID datapages.Subject `json:"instance_id" signal:"instance_id"`

	Input string `json:"input"`
	Fresh bool   `json:"fresh"`
}

type App struct {
	hmacKey [32]byte
}

func NewApp(hmacSecretKey [32]byte) *App {
	return &App{
		hmacKey: hmacSecretKey,
	}
}

// newID generates a crypto-random, HMAC-signed identifier.
// Format: base64url(random) "." base64url(hmac-sha256(random))
func (a *App) newID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf[:])
	mac := hmac.New(sha256.New, a.hmacKey[:])
	mac.Write(buf[:])
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return token + "~" + sig, nil
}

var (
	errInvalidID  = errors.New("invalid instance ID")
	errInvalidNum = errors.New("invalid num parameter")
	errInvalidBtn = errors.New("invalid btn parameter")
	numRe         = regexp.MustCompile(`^-?\d*\.?\d+$`)
)

func (a *App) verifyID(id string) error {
	parts := strings.SplitN(id, "~", 2)
	if len(parts) != 2 {
		return errInvalidID
	}
	token, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errInvalidID
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return errInvalidID
	}
	mac := hmac.New(sha256.New, a.hmacKey[:])
	mac.Write(token)
	if !hmac.Equal(mac.Sum(nil), sig) {
		return errInvalidID
	}
	return nil
}

func (*App) Head(_ *http.Request) datapages.Component { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (
	body datapages.Component,
	disableRefreshAfterHidden datapages.DisableRefreshAfterHidden,
	err error,
) {
	id, err := p.App.newID()
	if err != nil {
		return nil, true, err
	}
	return pageCalculator("", false, id), true, nil
}

// POSTInput is /input/{$}
func (p PageIndex) POSTInput(
	r *http.Request,
	calcUpdated datapages.Dispatcher[EventCalcUpdated],
	query datapages.Query[struct {
		Btn int    `query:"btn"`
		Num string `query:"num"`
	}],
	signals datapages.Signals[struct {
		InstanceID string `json:"instance_id"`
		Input      string `json:"input"`
		Fresh      bool   `json:"fresh"`
	}],
) error {
	if err := p.App.verifyID(signals.Values.InstanceID); err != nil {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, err)
	}

	if query.Values.Num != "" {
		if !numRe.MatchString(query.Values.Num) {
			return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidNum)
		}
		input := signals.Values.Input
		if signals.Values.Fresh {
			input = ""
		}
		return calcUpdated.Dispatch(EventCalcUpdated{
			InstanceID: datapages.Subject(signals.Values.InstanceID),
			Input:      input + query.Values.Num,
			Fresh:      false,
		})
	}
	btn := calc.CalcButton(query.Values.Btn)
	if !calc.ValidButton(btn) {
		return fmt.Errorf("%w: %w", datapages.ErrBadRequest, errInvalidBtn)
	}
	input, fresh := calc.Press(signals.Values.Input, signals.Values.Fresh, btn)
	return calcUpdated.Dispatch(EventCalcUpdated{
		InstanceID: datapages.Subject(signals.Values.InstanceID),
		Input:      input,
		Fresh:      fresh,
	})
}

func (PageIndex) OnCalcUpdated(
	event EventCalcUpdated,
	sse datapages.SSE,
) error {
	return sse.PatchElement(
		pageCalculator(event.Input, event.Fresh, string(event.InstanceID)),
	)
}
