// Package login walks a user through a network's login steps in the terminal.
package login

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mdp/qrterminal/v3"
	"golang.org/x/term"
	"maunium.net/go/mautrix/bridgev2"
)

// Terminal asks questions on stdin and prints on stdout.
type Terminal struct {
	in  *bufio.Reader
	out io.Writer
}

func NewTerminal() *Terminal {
	return &Terminal{in: bufio.NewReader(os.Stdin), out: os.Stdout}
}

// ChooseFlow lets the user pick a login method, e.g. "QR code" or "Phone number".
func (t *Terminal) ChooseFlow(flows []bridgev2.LoginFlow) (string, error) {
	if len(flows) == 0 {
		return "", errors.New("this network has no login methods")
	}
	if len(flows) == 1 {
		return flows[0].ID, nil
	}
	fmt.Fprintln(t.out, "Choose a login method:")
	for i, flow := range flows {
		fmt.Fprintf(t.out, "  %d) %s - %s\n", i+1, flow.Name, flow.Description)
	}
	for {
		answer, err := t.ask("Number: ", false)
		if err != nil {
			return "", err
		}
		n, err := strconv.Atoi(answer)
		if err == nil && n >= 1 && n <= len(flows) {
			return flows[n-1].ID, nil
		}
		fmt.Fprintln(t.out, "Please enter a number from the list.")
	}
}

// Run drives a login process until it completes, fails, or ctx is cancelled.
func (t *Terminal) Run(ctx context.Context, proc bridgev2.LoginProcess) (*bridgev2.UserLogin, error) {
	step, err := proc.Start(ctx)
	for err == nil {
		if step.Instructions != "" {
			fmt.Fprintln(t.out, "\n"+step.Instructions)
		}
		switch step.Type {
		case bridgev2.LoginStepTypeComplete:
			return step.CompleteParams.UserLogin, nil
		case bridgev2.LoginStepTypeDisplayAndWait:
			step, err = t.displayAndWait(ctx, proc, step.DisplayAndWaitParams)
		case bridgev2.LoginStepTypeUserInput:
			step, err = t.userInput(ctx, proc, step.UserInputParams)
		case bridgev2.LoginStepTypeCookies:
			step, err = t.cookies(ctx, proc, step.CookiesParams)
		default:
			err = fmt.Errorf("login step type %q is not supported in the terminal", step.Type)
		}
	}
	proc.Cancel()
	return nil, err
}

func (t *Terminal) displayAndWait(ctx context.Context, proc bridgev2.LoginProcess, params *bridgev2.LoginDisplayAndWaitParams) (*bridgev2.LoginStep, error) {
	waiter, ok := proc.(bridgev2.LoginProcessDisplayAndWait)
	if !ok {
		return nil, errors.New("login process cannot wait")
	}
	switch params.Type {
	case bridgev2.LoginDisplayTypeQR:
		fmt.Fprintln(t.out, "Scan this QR code with your phone:")
		qrterminal.GenerateHalfBlock(params.Data, qrterminal.L, t.out)
	case bridgev2.LoginDisplayTypeCode, bridgev2.LoginDisplayTypeEmoji:
		fmt.Fprintf(t.out, "Code: %s\n", params.Data)
	}
	fmt.Fprintln(t.out, "Waiting...")
	return waiter.Wait(ctx)
}

func (t *Terminal) userInput(ctx context.Context, proc bridgev2.LoginProcess, params *bridgev2.LoginUserInputParams) (*bridgev2.LoginStep, error) {
	submitter, ok := proc.(bridgev2.LoginProcessUserInput)
	if !ok {
		return nil, errors.New("login process cannot accept input")
	}
	values := make(map[string]string, len(params.Fields))
	for _, field := range params.Fields {
		value, err := t.askField(field)
		if err != nil {
			return nil, err
		}
		values[field.ID] = value
	}
	return submitter.SubmitUserInput(ctx, values)
}

func (t *Terminal) askField(field bridgev2.LoginInputDataField) (string, error) {
	if field.Description != "" {
		fmt.Fprintln(t.out, field.Description)
	}
	if len(field.Options) > 0 {
		fmt.Fprintf(t.out, "Options: %s\n", strings.Join(field.Options, ", "))
	}
	secret := field.Type == bridgev2.LoginInputFieldTypePassword || field.Type == bridgev2.LoginInputFieldTypeToken
	for {
		value, err := t.ask(field.Name+": ", secret)
		if err != nil {
			return "", err
		}
		if value == "" {
			value = field.DefaultValue
		}
		if field.Validate == nil {
			return value, nil
		}
		// The network's own check, e.g. "phone number must start with +".
		if cleaned, err := field.Validate(value); err == nil {
			return cleaned, nil
		} else {
			fmt.Fprintf(t.out, "Invalid: %v\n", err)
		}
	}
}

func (t *Terminal) cookies(ctx context.Context, proc bridgev2.LoginProcess, params *bridgev2.LoginCookiesParams) (*bridgev2.LoginStep, error) {
	submitter, ok := proc.(bridgev2.LoginProcessCookies)
	if !ok {
		return nil, errors.New("login process cannot accept cookies")
	}
	if browser := FindBrowser(); browser != "" && params.URL != "" {
		answer, err := t.ask(fmt.Sprintf("Log in with a browser window (%s)? [Y/n]: ", filepath.Base(browser)), false)
		if err != nil {
			return nil, err
		}
		if answer == "" || strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes") {
			values, err := CollectCookies(ctx, browser, params, t.out)
			if err == nil {
				return submitter.SubmitCookies(ctx, values)
			}
			fmt.Fprintf(t.out, "Browser login didn't work: %v\nYou can paste the values instead.\n", err)
		}
	}
	fmt.Fprintf(t.out, "Log in at %s in your browser, then paste these values from the developer tools.\n", params.URL)
	values := make(map[string]string, len(params.Fields))
	for _, field := range params.Fields {
		label := field.ID
		if !field.Required {
			label += " (optional, Enter to skip)"
		}
		value, err := t.ask(label+": ", true)
		if err != nil {
			return nil, err
		}
		if value == "" && field.Required {
			return nil, fmt.Errorf("%s is required", field.ID)
		}
		if value != "" {
			values[field.ID] = value
		}
	}
	return submitter.SubmitCookies(ctx, values)
}

// AskSecret asks for something like a password without showing what's typed.
func (t *Terminal) AskSecret(prompt string) (string, error) {
	return t.ask(prompt, true)
}

// ask prints a prompt and reads one line; secret input is not echoed on screen.
func (t *Terminal) ask(prompt string, secret bool) (string, error) {
	fmt.Fprint(t.out, prompt)
	if secret && term.IsTerminal(int(os.Stdin.Fd())) {
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(t.out)
		return strings.TrimSpace(string(raw)), err
	}
	line, err := t.in.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
