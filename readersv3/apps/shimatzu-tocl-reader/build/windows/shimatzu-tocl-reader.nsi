Unicode true
Name "WiseMED Shimatzu Tocl Reader"
OutFile "$%OUTPUT_EXE%"
InstallDir "$PROGRAMFILES64\WiseMED Shimatzu Tocl Reader"
RequestExecutionLevel admin
ShowInstDetails show
ShowUninstDetails show

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetOutPath "$INSTDIR"
  File /r "$%APP_PAYLOAD%\*.*"
  CreateShortcut "$SMPROGRAMS\WiseMED\WiseMED Shimatzu Tocl Reader.lnk" "$INSTDIR\shimatzu-tocl-reader.exe"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\install-service.ps1"'
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-service.ps1"'
  Delete "$SMPROGRAMS\WiseMED\WiseMED Shimatzu Tocl Reader.lnk"
  RMDir /r "$INSTDIR"
SectionEnd
