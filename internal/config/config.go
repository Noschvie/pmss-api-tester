package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
)

const defaultHTTPAddr = ":8080"

type Config struct {
	HTTPAddr string
}

func Load() (Config, error) {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = defaultHTTPAddr
	}

	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR %q: %w", addr, err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR port %q: expected a number between 1 and 65535", portText)
	}

	return Config{HTTPAddr: addr}, nil
}
