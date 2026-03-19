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

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/romshark/datapages/example/calculator/app/calc"
	"github.com/romshark/datapages/example/calculator/datapagesgen/httperr"
)

// EventCalcUpdated is "calc.updated"
type EventCalcUpdated struct {
	SubjectInstanceID string `json:"subject_id" signal:"instance_id"`

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

func (*App) Head(_ *http.Request) templ.Component { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (
	body templ.Component,
	disableRefreshAfterHidden bool,
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
	dispatch func(EventCalcUpdated) error,
	query struct {
		Btn int    `query:"btn"`
		Num string `query:"num"`
	},
	signals struct {
		InstanceID string `json:"instance_id"`
		Input      string `json:"input"`
		Fresh      bool   `json:"fresh"`
	},
) error {
	if err := p.App.verifyID(signals.InstanceID); err != nil {
		return fmt.Errorf("%w: %w", httperr.BadRequest, err)
	}

	if query.Num != "" {
		if !numRe.MatchString(query.Num) {
			return fmt.Errorf("%w: %w", httperr.BadRequest, errInvalidNum)
		}
		input := signals.Input
		if signals.Fresh {
			input = ""
		}
		return dispatch(EventCalcUpdated{
			SubjectInstanceID: signals.InstanceID,
			Input:             input + query.Num,
			Fresh:             false,
		})
	}
	btn := calc.CalcButton(query.Btn)
	if !calc.ValidButton(btn) {
		return fmt.Errorf("%w: %w", httperr.BadRequest, errInvalidBtn)
	}
	input, fresh := calc.Press(signals.Input, signals.Fresh, btn)
	return dispatch(EventCalcUpdated{
		SubjectInstanceID: signals.InstanceID,
		Input:             input,
		Fresh:             fresh,
	})
}

func (PageIndex) OnCalcUpdated(
	event EventCalcUpdated,
	sse *datastar.ServerSentEventGenerator,
) error {
	return sse.PatchElementTempl(
		pageCalculator(event.Input, event.Fresh, event.SubjectInstanceID),
	)
}
