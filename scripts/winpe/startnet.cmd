@echo off
setlocal
set LOG=X:\vmlite-winpe.log
echo vmlite winpe start > %LOG%

wpeinit >> %LOG% 2>&1

rem The primary VM disk is disk 0; the marker disk is disk 1.
(
echo select disk 1
echo clean
echo create partition primary
echo format fs=fat32 label=VMLITE quick
echo assign letter=M
) > X:\vmlite-diskpart.txt

diskpart /s X:\vmlite-diskpart.txt >> %LOG% 2>&1
if errorlevel 1 goto fail

echo winpe-ok> M:\vmlite-winpe-ok.txt
echo vmlite winpe marker written >> %LOG%
ping -n 3 127.0.0.1 >nul
wpeutil shutdown
exit /b 0

:fail
echo vmlite winpe diskpart failed >> %LOG%
wpeutil shutdown
exit /b 1
