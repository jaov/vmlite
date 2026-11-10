package installer

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"fmt"
	"strings"
	"text/template"
)

//go:embed windows_autounattend.xml.tmpl
var windowsAutoUnattendTemplate string

// WindowsOptions controls a generated autounattend.xml.
type WindowsOptions struct {
	Locale        string
	InputLocale   string
	Timezone      string
	ImageIndex    int
	AdminPassword string
	Firmware      string
}

type windowsTemplateData struct {
	Hostname      string
	InputLocale   string
	SystemLocale  string
	UILanguage    string
	UserLocale    string
	Timezone      string
	ImageIndex    int
	AdminPassword string
	SSHKey        string
	UEFI          bool
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func powershellQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func renderWindowsAutoUnattend(in Input) ([]byte, error) {
	opts := WindowsOptions{}
	if in.Windows != nil {
		opts = *in.Windows
	}
	if strings.TrimSpace(opts.Locale) == "" {
		opts.Locale = "en-US"
	}
	if strings.TrimSpace(opts.InputLocale) == "" {
		opts.InputLocale = "0409:00000409"
	}
	if strings.TrimSpace(opts.Timezone) == "" {
		opts.Timezone = "UTC"
	}
	if opts.ImageIndex <= 0 {
		opts.ImageIndex = 1
	}
	if strings.TrimSpace(opts.AdminPassword) == "" {
		return nil, fmt.Errorf("generated windows autounattend requires windows.admin_password")
	}
	if strings.TrimSpace(in.SSHKey) == "" {
		return nil, fmt.Errorf("generated windows autounattend requires an SSH authorized key")
	}

	data := windowsTemplateData{
		Hostname:      in.Hostname,
		InputLocale:   opts.InputLocale,
		SystemLocale:  opts.Locale,
		UILanguage:    opts.Locale,
		UserLocale:    opts.Locale,
		Timezone:      opts.Timezone,
		ImageIndex:    opts.ImageIndex,
		AdminPassword: opts.AdminPassword,
		SSHKey:        in.SSHKey,
		UEFI:          strings.EqualFold(opts.Firmware, "uefi"),
	}

	tmpl, err := template.New("windows-autounattend").
		Funcs(template.FuncMap{
			"xml": xmlEscape,
			"ps":  powershellQuote,
		}).
		Parse(windowsAutoUnattendTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse windows autounattend template: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("render windows autounattend: %w", err)
	}
	return out.Bytes(), nil
}
