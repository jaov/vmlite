#requires -RunAsAdministrator
<#
.SYNOPSIS
Build a small WinPE ISO for vmlite CI.

.DESCRIPTION
Uses the Windows ADK and WinPE add-on to build a WinPE ISO whose
startnet.cmd writes a marker to the second disk and powers the VM off.
The resulting ISO is a CI artifact; do not commit or redistribute it.

Requires:
  - Windows ADK
  - Windows PE add-on for the ADK
  - an elevated PowerShell session

Example:
  .\scripts\build-winpe.ps1 -Output .\artifacts\winpe-vmlite.iso
#>
param(
    [string]$Arch = "amd64",
    [string]$Output = (Join-Path $PSScriptRoot "..\artifacts\winpe-vmlite.iso"),
    [string]$Work = (Join-Path $env:TEMP "vmlite-winpe")
)

$ErrorActionPreference = "Stop"

$programFilesX86 = ${env:ProgramFiles(x86)}
if (-not $programFilesX86) {
    $programFilesX86 = $env:ProgramFiles
}
$adk = Join-Path $programFilesX86 "Windows Kits\10\Assessment and Deployment Kit"
$winpe = Join-Path $adk "Windows Preinstallation Environment"
$copype = Join-Path $winpe "$Arch\copype.cmd"
$makeMedia = Join-Path $winpe "$Arch\MakeWinPEMedia.cmd"

if (-not (Test-Path $copype)) {
    throw "WinPE ADK files not found at $copype. Install the Windows ADK and WinPE add-on."
}
if (-not (Test-Path $makeMedia)) {
    throw "MakeWinPEMedia.cmd not found at $makeMedia."
}

$startnet = Join-Path $PSScriptRoot "winpe\startnet.cmd"
if (-not (Test-Path $startnet)) {
    throw "startnet.cmd not found at $startnet."
}

if (Test-Path $Work) {
    Remove-Item -Recurse -Force $Work
}
$Output = [System.IO.Path]::GetFullPath($Output)
$outputDir = Split-Path -Parent $Output
New-Item -ItemType Directory -Force $outputDir | Out-Null

Write-Host "Creating WinPE media in $Work"
& cmd.exe /c "`"$copype`" $Arch `"$Work`""
if ($LASTEXITCODE -ne 0) {
    throw "copype failed with exit code $LASTEXITCODE"
}

$wim = Join-Path $Work "media\sources\boot.wim"
$mount = Join-Path $Work "mount"
New-Item -ItemType Directory -Force $mount | Out-Null

Write-Host "Mounting $wim"
& dism.exe /Mount-Wim /WimFile:$wim /Index:1 /MountDir:$mount
if ($LASTEXITCODE -ne 0) {
    throw "dism mount failed with exit code $LASTEXITCODE"
}

try {
    Copy-Item $startnet (Join-Path $mount "Windows\System32\startnet.cmd") -Force
}
finally {
    Write-Host "Committing $wim"
    & dism.exe /Unmount-Wim /MountDir:$mount /Commit
    if ($LASTEXITCODE -ne 0) {
        throw "dism unmount failed with exit code $LASTEXITCODE"
    }
}

Write-Host "Building ISO at $Output"
& cmd.exe /c "`"$makeMedia`" /ISO `"$Work`" `"$Output`""
if ($LASTEXITCODE -ne 0) {
    throw "MakeWinPEMedia failed with exit code $LASTEXITCODE"
}

Write-Host "Built $Output"
