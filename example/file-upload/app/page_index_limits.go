package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/romshark/datapages"
)

// LimitSignals are the transfer limits as the controls of the page hold them:
// a rate in KiB/s per direction and whether its switch says unlimited.
// [PageIndex.POSTLimits] reads the signals that [PageIndex.OnLimitsChanged] writes back.
type LimitSignals struct {
	Upload            LimitKiB `json:"limitUp"`
	Download          LimitKiB `json:"limitDown"`
	UploadUnlimited   bool     `json:"unlimitedUp"`
	DownloadUnlimited bool     `json:"unlimitedDown"`
}

// LimitKiB is a rate as a number field reports it.
//
// The bound field sends a JSON number and an emptied one sends zero.
// A quoted rate and an empty string are read too: Datastar binds a web
// component by its value property, which is a string, so a field that becomes
// one still reports a rate. A rate of nothing is not a rate: it becomes zero,
// which [clampLimit] answers with the minimum. Refusing it would leave the
// field holding a value the server never took.
type LimitKiB int64

func (l *LimitKiB) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if unquoted, err := strconv.Unquote(s); err == nil {
		s = strings.TrimSpace(unquoted)
	}
	if s == "" || s == "null" {
		*l = 0
		return nil
	}
	kib, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("%w: reading a rate: %w", datapages.ErrBadRequest, err)
	}
	*l = LimitKiB(kib)
	return nil
}

// limitSignalsOf is what the controls show for the limits that took effect.
func limitSignalsOf(l Limits) LimitSignals {
	return LimitSignals{
		Upload:            LimitKiB(l.Upload.KiB),
		Download:          LimitKiB(l.Download.KiB),
		UploadUnlimited:   l.Upload.Unlimited,
		DownloadUnlimited: l.Download.Unlimited,
	}
}

// POSTLimits is /limits
//
// The browser only says what the limits should be. Every byte is paced on the server,
// in [PageIndex.PUTChunk] and in [Downloads].
func (p PageIndex) POSTLimits(
	r *http.Request,
	signals datapages.Signals[LimitSignals],
	limitsChanged datapages.Dispatcher[EventLimitsChanged],
) error {
	p.App.SetLimits(Limits{
		Upload: Limit{
			KiB:       int64(signals.Values.Upload),
			Unlimited: signals.Values.UploadUnlimited,
		},
		Download: Limit{
			KiB:       int64(signals.Values.Download),
			Unlimited: signals.Values.DownloadUnlimited,
		},
	})
	return limitsChanged.Dispatch(EventLimitsChanged{})
}

// OnLimitsChanged writes the limits that took effect back into the inputs of every tab.
// It patches signals rather than the element holding the inputs,
// which would replace what someone is typing mid-keystroke.
//
// It also resizes the running transfers, which would otherwise keep the chunk
// size they started with until their next resume.
func (p PageIndex) OnLimitsChanged(
	event EventLimitsChanged, sse datapages.SSE,
) error {
	if err := sse.PatchSignals(limitSignalsOf(p.App.Limits())); err != nil {
		return err
	}
	script, err := callScript("dpUpload.resize", p.App.ChunkSize())
	if err != nil {
		return err
	}
	return sse.ExecuteScript(script)
}

// limitStepExpr steps the rate signal to the next multiple of [LimitStepKiB],
// up or down, floored at [MinLimitKiB].
//
// Snapping rather than adding, so that the rates a visitor lands on are round.
// Adding would inherit the rule of a native number field, whose steps count from min:
// with a minimum of 1 the field offers 1, 65, 129, and one step down
// from 64 to the floor leaves every later step off the round grid.
func limitStepExpr(rate string, up bool) string {
	step, n := strconv.Itoa(LimitStepKiB), "-1"
	round := "Math.ceil"
	if up {
		n, round = "+1", "Math.floor"
	}
	next := "(" + round + "(Number($" + rate + ")/" + step + ")" + n + ")*" + step
	return "$" + rate + " = Math.min(" + strconv.Itoa(MaxLimitKiB) +
		", Math.max(" + strconv.Itoa(MinLimitKiB) + ", " + next + "))"
}
