package claudecli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const (
	maxProcessErrorBytes = 64 << 10
	maxStreamLineBytes   = 16 << 20
)

type commandSpec struct {
	Binary string
	Args   []string
	Dir    string
	Env    []string
	Stdin  string
}

type commandRunner interface {
	Run(context.Context, commandSpec, func([]byte) error) error
}

type processRunner struct{}

func newProcessRunner() commandRunner { return processRunner{} }

func (processRunner) Run(ctx context.Context, spec commandSpec, onLine func([]byte) error) error {
	if strings.TrimSpace(spec.Binary) == "" {
		return errors.New("command binary is required")
	}
	binary, args, launcher, err := commandInvocation(spec)
	if err != nil {
		return err
	}
	command := exec.Command(binary, args...)
	command.Dir = spec.Dir
	command.Env = spec.Env
	command.Stdin = strings.NewReader(spec.Stdin)
	configureProcess(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open command stdout: %w", err)
	}
	var stderr limitedBuffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	cancelWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = terminateProcessTree(command.Process, launcher)
		case <-cancelWatch:
		}
	}()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxStreamLineBytes)
	var streamErr error
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		if err := onLine(line); err != nil {
			streamErr = err
			_ = terminateProcessTree(command.Process, launcher)
			break
		}
	}
	if streamErr == nil {
		streamErr = scanner.Err()
	}
	waitErr := command.Wait()
	close(cancelWatch)
	if ctx.Err() != nil {
		return fmt.Errorf("command cancelled: %w", ctx.Err())
	}
	if streamErr != nil {
		return streamErr
	}
	if waitErr != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			return fmt.Errorf("command exit: %w", waitErr)
		}
		return fmt.Errorf("command exit: %w: %s", waitErr, message)
	}
	return nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := maxProcessErrorBytes - b.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = b.Buffer.Write(data)
	}
	return original, nil
}

var _ io.Writer = (*limitedBuffer)(nil)
