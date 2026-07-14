Unicode true
Name "WiseMED Snibe Maglumi X3"
OutFile "$%OUTPUT_EXE%"
InstallDir "$PROGRAMFILES64\WiseMED Snibe Maglumi X3"
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
  CreateShortcut "$SMPROGRAMS\WiseMED\WiseMED Snibe Maglumi X3.lnk" "$INSTDIR\snibe-maglumi-x3.exe"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\install-service.ps1"'
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-service.ps1"'
  Delete "$SMPROGRAMS\WiseMED\WiseMED Snibe Maglumi X3.lnk"
  RMDir /r "$INSTDIR"
SectionEnd
