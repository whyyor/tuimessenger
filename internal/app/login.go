package app

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/random"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/status"

	"github.com/whyyor/tuimessenger/internal/csapi"
)

// loginTimeout drops a login nobody finished, like mautrix's own provisioning API.
const loginTimeout = 30 * time.Minute

// loginSession is one login in progress. Its steps run on its own context, not the HTTP request's, so a dropped
// connection doesn't break a login halfway.
type loginSession struct {
	id       string
	network  string
	proc     bridgev2.LoginProcess
	override *bridgev2.UserLogin // the network's current login, replaced by this one
	ctx      context.Context
	cancel   context.CancelFunc
	endOnce  sync.Once

	mu   sync.Mutex // one step at a time
	step *bridgev2.LoginStep
}

var (
	errLoginGone  = mautrix.MNotFound.WithMessage("This login has ended; start it again")
	errStaleStep  = mautrix.RespError{ErrCode: "M_BAD_STATE", Err: "This login step isn't current anymore; start the login again", StatusCode: http.StatusConflict}
	errNoNetwork  = mautrix.MNotFound.WithMessage("No such network is running")
	errNilStep    = errors.New("the network didn't say what comes next")
	errNotAllowed = mautrix.MForbidden.WithMessage("This network doesn't support that step")
)

// loginError keeps a network's own message (e.g. "invalid code") for the user instead of a generic internal error.
func loginError(err error) error {
	var respErr mautrix.RespError
	var bridgeErr bridgev2.RespError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &respErr):
		return respErr
	case errors.As(err, &bridgeErr):
		return mautrix.RespError(bridgeErr)
	default:
		return mautrix.MUnknown.WithMessage(err.Error())
	}
}

// StartLogin begins logging in to a network with one of its login flows. A network has one account: logging in again
// replaces the current login.
func (a *App) StartLogin(ctx context.Context, network, flowID string) (*csapi.LoginStep, error) {
	br := a.Bridge(network)
	if br == nil {
		return nil, errNoNetwork
	}
	user, err := br.GetUserByMXID(ctx, a.opts.Owner)
	if err != nil {
		return nil, loginError(err)
	}
	proc, err := br.Network.CreateLogin(ctx, user, flowID)
	if err != nil {
		return nil, loginError(err)
	}
	sctx, cancel := context.WithTimeout(br.BackgroundCtx, loginTimeout)
	s := &loginSession{id: random.String(16), network: network, proc: proc, ctx: sctx, cancel: cancel}
	if logins := user.GetUserLogins(); len(logins) > 0 {
		s.override = logins[0]
	}
	s.ctx = zerolog.Ctx(ctx).With().Str("login_session", s.id).Str("network", network).Logger().WithContext(sctx)
	a.loginsLock.Lock()
	if a.logins == nil {
		a.logins = make(map[string]*loginSession)
	}
	a.logins[s.id] = s
	a.loginsLock.Unlock()
	go func() {
		<-sctx.Done()
		if errors.Is(sctx.Err(), context.DeadlineExceeded) {
			a.endLogin(s, true)
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	var step *bridgev2.LoginStep
	if withParams, ok := proc.(bridgev2.LoginProcessWithParams); ok {
		step, err = withParams.StartWithParams(s.ctx, bridgev2.LoginStartParams{Override: s.override})
	} else if withOverride, ok := proc.(bridgev2.LoginProcessWithOverride); ok && s.override != nil {
		step, err = withOverride.StartWithOverride(s.ctx, s.override)
	} else {
		step, err = proc.Start(s.ctx)
	}
	return a.afterStep(s, step, err)
}

// SubmitLogin answers the current step of a login: form values, cookies, or (for a step that shows a QR code or a
// code) nothing, which waits until the network moves on. It returns the next step.
func (a *App) SubmitLogin(ctx context.Context, loginID, stepID string, stepType bridgev2.LoginStepType, input map[string]string) (*csapi.LoginStep, error) {
	s := a.loginSession(loginID)
	if s == nil {
		return nil, errLoginGone
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.step == nil || s.step.StepID != stepID || s.step.Type != stepType {
		return nil, errStaleStep
	}
	var next *bridgev2.LoginStep
	var err error
	switch stepType {
	case bridgev2.LoginStepTypeUserInput:
		submitter, ok := s.proc.(bridgev2.LoginProcessUserInput)
		if !ok {
			return nil, errNotAllowed
		}
		values, verr := validateLoginInput(s.step.UserInputParams, input)
		if verr != nil {
			return nil, verr // the user can fix it; the login goes on
		}
		next, err = submitter.SubmitUserInput(s.ctx, values)
	case bridgev2.LoginStepTypeCookies:
		submitter, ok := s.proc.(bridgev2.LoginProcessCookies)
		if !ok {
			return nil, errNotAllowed
		}
		if verr := checkCookies(s.step.CookiesParams, input); verr != nil {
			return nil, verr
		}
		next, err = submitter.SubmitCookies(s.ctx, input)
	case bridgev2.LoginStepTypeDisplayAndWait:
		waiter, ok := s.proc.(bridgev2.LoginProcessDisplayAndWait)
		if !ok {
			return nil, errNotAllowed
		}
		next, err = waiter.Wait(s.ctx)
	default:
		return nil, errNotAllowed
	}
	return a.afterStep(s, next, err)
}

// afterStep keeps the login going with the next step, finishes it, or ends it on an error (which is always final).
func (a *App) afterStep(s *loginSession, step *bridgev2.LoginStep, err error) (*csapi.LoginStep, error) {
	if err == nil && step == nil {
		err = errNilStep
	}
	if err == nil {
		switch step.Type {
		case bridgev2.LoginStepTypeUserInput, bridgev2.LoginStepTypeCookies, bridgev2.LoginStepTypeDisplayAndWait, bridgev2.LoginStepTypeComplete:
		default:
			err = mautrix.MUnrecognized.WithMessage("This way of logging in (%s) isn't supported here; try another one", step.Type)
		}
	}
	if err != nil {
		zerolog.Ctx(s.ctx).Warn().Err(err).Msg("Login failed")
		a.endLogin(s, true)
		return nil, loginError(err)
	}
	step.TxnID = random.String(12)
	if step.Type == bridgev2.LoginStepTypeComplete {
		a.finishLogin(s, step)
	} else {
		s.step = step
	}
	return &csapi.LoginStep{LoginID: s.id, Step: step}, nil
}

// finishLogin removes the replaced login if the new one is a different account.
func (a *App) finishLogin(s *loginSession, step *bridgev2.LoginStep) {
	zerolog.Ctx(s.ctx).Info().Str("user_login_id", string(step.CompleteParams.UserLoginID)).Msg("Logged in")
	if s.override != nil && step.CompleteParams != nil && s.override.ID != step.CompleteParams.UserLoginID {
		s.override.Delete(s.ctx, status.BridgeState{StateEvent: status.StateLoggedOut, Reason: "LOGIN_OVERRIDDEN"},
			bridgev2.DeleteOpts{LogoutRemote: true})
	}
	a.endLogin(s, false)
}

// CancelLogin stops a login in progress.
func (a *App) CancelLogin(ctx context.Context, loginID string) error {
	s := a.loginSession(loginID)
	if s == nil {
		return errLoginGone
	}
	a.endLogin(s, true)
	return nil
}

func (a *App) loginSession(id string) *loginSession {
	a.loginsLock.Lock()
	defer a.loginsLock.Unlock()
	return a.logins[id]
}

// endLogin forgets a login; cancelProc also tells the network to stop (not after a successful login).
func (a *App) endLogin(s *loginSession, cancelProc bool) {
	s.endOnce.Do(func() {
		a.loginsLock.Lock()
		delete(a.logins, s.id)
		a.loginsLock.Unlock()
		if cancelProc {
			s.proc.Cancel()
		}
		s.cancel()
	})
}

// validateLoginInput checks form values the way the network asks (required fields, phone numbers, patterns) and
// returns them cleaned up, e.g. "+1 555 010-0001" → "+15550100001".
func validateLoginInput(params *bridgev2.LoginUserInputParams, input map[string]string) (map[string]string, error) {
	if params == nil {
		return nil, errNotAllowed
	}
	values := make(map[string]string, len(params.Fields))
	for _, field := range params.Fields {
		field.FillDefaultValidate()
		value := input[field.ID]
		if value == "" {
			value = field.DefaultValue
		}
		if value == "" && !field.Optional {
			return nil, mautrix.MInvalidParam.WithMessage("%s is required", field.Name)
		}
		cleaned, err := field.Validate(value)
		if err != nil {
			return nil, mautrix.MInvalidParam.WithMessage("%s: %v", field.Name, err)
		}
		values[field.ID] = cleaned
	}
	return values, nil
}

// checkCookies makes sure every required value from the browser is there.
func checkCookies(params *bridgev2.LoginCookiesParams, input map[string]string) error {
	if params == nil {
		return errNotAllowed
	}
	for _, field := range params.Fields {
		if field.Required && input[field.ID] == "" {
			return mautrix.MInvalidParam.WithMessage("%s is missing; log in in the browser again", field.ID)
		}
	}
	return nil
}

// Logout logs the network's account out, on the network too.
func (a *App) Logout(ctx context.Context, network string, loginID networkid.UserLoginID) error {
	br := a.Bridge(network)
	if br == nil {
		return errNoNetwork
	}
	login := br.GetCachedUserLoginByID(loginID)
	if login == nil {
		return mautrix.MNotFound.WithMessage("Not logged in to %s with that account", network)
	}
	login.Logout(ctx)
	return nil
}
