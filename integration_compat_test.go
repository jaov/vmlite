package main

import (
	"crypto/sha256"
	"encoding/hex"

	"vmlite/internal/config"
	"vmlite/internal/host"
	"vmlite/internal/state"
)

// Compatibility shims for the root-package integration tests. The CLI and
// domain code now live in internal packages.
type Config = config.Config
type TPM = config.TPM
type Source = config.Source
type Installer = config.Installer
type WindowsInstaller = config.WindowsInstaller
type Disk = config.Disk
type Mount = config.Mount
type Network = config.Network
type Graphics = config.Graphics
type ExtraDisk = config.ExtraDisk
type SSHConfig = config.SSHConfig
type Provisioner = config.Provisioner
type Upload = config.Upload

const (
	sourceNone        = config.SourceNone
	sourceCloudImage  = config.SourceCloudImage
	sourceInstallTree = config.SourceInstallTree
	sourceISO         = config.SourceISO
	sourceDisk        = config.SourceDisk
	sourcePXE         = config.SourcePXE

	firmwareBIOS = config.FirmwareBIOS
	firmwareUEFI = config.FirmwareUEFI

	mountType9P       = config.MountType9P
	mountTypeNone     = config.MountTypeNone
	mountTypeVirtioFS = config.MountTypeVirtioFS

	networkModeAuto    = config.NetworkModeAuto
	networkModeDefault = config.NetworkModeDefault
	networkModeUser    = config.NetworkModeUser
	networkModeBridge  = config.NetworkModeBridge

	graphicsNone  = config.GraphicsNone
	graphicsVNC   = config.GraphicsVNC
	graphicsSPICE = config.GraphicsSPICE

	diskFormatAuto  = config.DiskFormatAuto
	diskFormatQCOW2 = config.DiskFormatQCOW2
	diskFormatRaw   = config.DiskFormatRaw

	diskBusAuto   = config.DiskBusAuto
	diskBusVirtio = config.DiskBusVirtio
	diskBusSATA   = config.DiskBusSATA
	diskBusSCSI   = config.DiskBusSCSI

	tpmBackendEmulator = config.TPMBackendEmulator
	tpmVersion12       = config.TPMVersion12
	tpmVersion20       = config.TPMVersion20

	systemURI  = host.SystemURI
	sessionURI = host.SessionURI

	installPhaseInstalled = state.PhaseInstalled
)

func ValidateConnection(uri string) error {
	return host.ValidateConnection(uri)
}

func loadState(path string) (*state.State, error) {
	return state.Load(path)
}

func shaHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
