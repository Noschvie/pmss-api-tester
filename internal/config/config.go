package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const defaultHTTPAddr = ":8080"

type Config struct {
	HTTPAddr       string
	PMSSAPIBaseURL string
	OIDC           OIDCConfig
}

type OIDCConfig struct {
	IssuerURL    string
	InternalURL  string
	ClientID     string
	ClientSecret string
	CallbackURL  string
	Audience     string
	CookieSecure bool
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

	oidcConfig := OIDCConfig{
		IssuerURL:    strings.TrimRight(os.Getenv("OIDC_ISSUER_URL"), "/"),
		InternalURL:  strings.TrimRight(os.Getenv("OIDC_INTERNAL_URL"), "/"),
		ClientID:     os.Getenv("OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("OIDC_CLIENT_SECRET"),
		CallbackURL:  os.Getenv("OIDC_CALLBACK_URL"),
		Audience:     os.Getenv("PMSS_AUDIENCE"),
		CookieSecure: true,
	}
	if oidcConfig.CallbackURL == "" {
		oidcConfig.CallbackURL = "http://localhost:8080/auth/callback"
	}
	if oidcConfig.Audience == "" {
		oidcConfig.Audience = "phylax-mqtt-security-service"
	}
	if secure := os.Getenv("OIDC_COOKIE_SECURE"); secure != "" {
		oidcConfig.CookieSecure, err = strconv.ParseBool(secure)
		if err != nil {
			return Config{}, fmt.Errorf("invalid OIDC_COOKIE_SECURE %q: expected true or false", secure)
		}
	}

	configuredOIDC := oidcConfig.IssuerURL != "" || oidcConfig.ClientID != "" || oidcConfig.ClientSecret != ""
	if configuredOIDC {
		if oidcConfig.IssuerURL == "" || oidcConfig.ClientID == "" || oidcConfig.ClientSecret == "" {
			return Config{}, fmt.Errorf("OIDC_ISSUER_URL, OIDC_CLIENT_ID, and OIDC_CLIENT_SECRET must all be configured")
		}
		for name, value := range map[string]string{
			"OIDC_ISSUER_URL":   oidcConfig.IssuerURL,
			"OIDC_CALLBACK_URL": oidcConfig.CallbackURL,
		} {
			if err := validateHTTPURL(name, value); err != nil {
				return Config{}, err
			}
		}
		if oidcConfig.InternalURL != "" {
			if err := validateHTTPURL("OIDC_INTERNAL_URL", oidcConfig.InternalURL); err != nil {
				return Config{}, err
			}
		}
		if strings.TrimSpace(oidcConfig.Audience) == "" {
			return Config{}, fmt.Errorf("PMSS_AUDIENCE must not be empty")
		}
	}

	pmssAPIBaseURL := strings.TrimRight(os.Getenv("PMSS_API_BASE_URL"), "/")
	if pmssAPIBaseURL != "" {
		if err := validateHTTPSURL("PMSS_API_BASE_URL", pmssAPIBaseURL); err != nil {
			return Config{}, err
		}
	}

	return Config{HTTPAddr: addr, PMSSAPIBaseURL: pmssAPIBaseURL, OIDC: oidcConfig}, nil
}

func validateHTTPURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid %s %q: expected an absolute HTTP or HTTPS URL", name, value)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("invalid %s %q: user info, query, and fragment are not allowed", name, value)
	}
	return nil
}

func validateHTTPSURL(name, value string) error {
	if err := validateHTTPURL(name, value); err != nil {
		return err
	}
	parsed, _ := url.Parse(value)
	if parsed.Scheme != "https" {
		return fmt.Errorf("invalid %s %q: HTTPS is required", name, value)
	}
	return nil
}
