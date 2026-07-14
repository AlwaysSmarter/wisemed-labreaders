Unicode true
Name "WiseMED Signing Pad Utility"
OutFile "$%OUTPUT_EXE%"
InstallDir "$PROGRAMFILES64\WiseMED Signing Pad Utility"
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
  CreateShortcut "$SMPROGRAMS\WiseMED\WiseMED Signing Pad Utility.lnk" "$INSTDIR\signing-pad-utility.exe"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\install-service.ps1"'
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-service.ps1"'
  Delete "$SMPROGRAMS\WiseMED\WiseMED Signing Pad Utility.lnk"
  RMDir /r "$INSTDIR"
SectionEnd
