package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func connectOnly(test *testing.T, proxyAddress string) {
	_, err := net.Dial("tcp", proxyAddress)
	if err != nil {
		test.Error(err)
		return
	}
	//give large-model-proxy time to start the service, so that it doesn't get killed before it started it
	//which can lead to false positive passing tests
	time.Sleep(1 * time.Second)
}

func minimal(test *testing.T, proxyAddress string) {
	conn, err := net.Dial("tcp", proxyAddress)
	if err != nil {
		test.Error(err)
		return
	}

	buffer := make([]byte, 1024)
	bytesRead, err := conn.Read(buffer)
	if err != nil {
		if err != io.EOF {
			test.Error(err)
			return
		}
	}
	pidString := string(buffer[:bytesRead])
	if !isNumeric(pidString) {
		test.Errorf("value \"%s\" is not numeric, expected a pid", pidString)
		return
	}
	pidInt, err := strconv.Atoi(pidString)
	if err != nil {
		test.Error(err, pidString)
		return
	}
	if pidInt <= 0 {
		test.Errorf("value \"%s\" is not a valid pid", pidString)
		return
	}
	if !isProcessRunning(pidInt) {
		test.Errorf("process \"%s\" is not running while connection is still open", pidString)
		return
	}

	err = conn.Close()
	if err != nil {
		test.Error(err)
		return
	}
}

func isNumeric(s string) bool {
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
func isProcessRunning(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
func startLargeModelProxy(testCaseName string, configPath string, waitChannel chan error) (*exec.Cmd, error) {
	cmd := exec.Command("./large-model-proxy", "-c", configPath)
	logFilePath := fmt.Sprintf("logs/test_%s.log", testCaseName)
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	} else {
		log.Printf("Failed to open log file for test %s", logFilePath)
	}
	if err := cmd.Start(); err != nil {
		waitChannel <- err
		return nil, err
	}
	go func() {
		waitChannel <- cmd.Wait()
	}()

	time.Sleep(1 * time.Second)

	select {
	case err := <-waitChannel:
		if err != nil {
			return nil, fmt.Errorf("large-model-proxy exited prematurely with error %v", err)
		} else {
			return nil, fmt.Errorf("large-model-proxy exited prematurely with success")
		}
	default:
	}

	err = cmd.Process.Signal(syscall.Signal(0))
	if err != nil {
		if err.Error() == "os: process already finished" {
			return nil, fmt.Errorf("large-model-proxy exited prematurely")
		}
		return nil, fmt.Errorf("error checking process state: %w", err)
	}

	return cmd, nil
}

func stopApplication(cmd *exec.Cmd, waitChannel chan error) error {
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		return err
	}

	select {
	case err := <-waitChannel:
		if err != nil && err.Error() != "waitid: no child processes" && err.Error() != "wait: no child processes" {
			return err
		}
		return nil
	case <-time.After(15 * time.Second):
		// Optionally kill the process if it hasn't exited
		_ = cmd.Process.Kill()
		return errors.New("large-model-proxy process did not stop within 15 seconds after receiving SIGINT")
	}
}
func checkPortClosed(port string) error {
	_, err := net.DialTimeout("tcp", net.JoinHostPort("localhost", port), time.Second)
	if err == nil {
		return fmt.Errorf("port %s is still open", port)
	}
	return nil
}

func TestAppScenarios(test *testing.T) {

	tests := []struct {
		Name       string
		ConfigPath string
		Port       string
		TestFunc   func(t *testing.T, proxyAddress string)
	}{
		{"minimal", "test-server/minimal.json", "2000", minimal},
		{
			"healthcheck",
			"test-server/healthcheck.json",
			"2001",
			minimal,
		},
		{
			"healthcheck-immediate-listen-start",
			"test-server/healthcheck-immediate-listen-start.json",
			"2002",
			minimal,
		},
		{
			"healthcheck-immediate-startup-delayed-healthcheck",
			"test-server/healthcheck-immediate-startup-delayed-healthcheck.json",
			"2003",
			minimal,
		},
		{
			"healthcheck-immediate-startup",
			"test-server/healthcheck-immediate-startup.json",
			"2004",
			minimal,
		},
		{
			"healthcheck-stuck",
			"test-server/healthcheck-stuck.json",
			"2005",
			connectOnly,
		},
		{
			"service-stuck-no-healthcheck",
			"test-server/service-stuck-no-healthcheck.json",
			"2006",
			connectOnly,
		},
	}

	for _, testCase := range tests {
		testCase := testCase
		test.Run(testCase.Name, func(test *testing.T) {
			test.Parallel()
			var waitChannel = make(chan error, 1)
			cmd, err := startLargeModelProxy(testCase.Name, testCase.ConfigPath, waitChannel)
			if err != nil {
				test.Fatalf("could not start application: %v", err)
			}

			defer func(cmd *exec.Cmd, port string, waitChannel chan error) {
				if cmd == nil {
					test.Errorf("not stopping application since there was a start error: %v", err)
					return
				}
				if err := stopApplication(cmd, waitChannel); err != nil {
					test.Errorf("failed to stop application: %v", err)
				}
				if err := checkPortClosed(port); err != nil {
					test.Errorf("port %s is still open after application exit: %v", port, err)
				}
			}(cmd, testCase.Port, waitChannel)

			proxyAddress := fmt.Sprintf("localhost:%s", testCase.Port)
			testCase.TestFunc(test, proxyAddress)
		})
	}
}
