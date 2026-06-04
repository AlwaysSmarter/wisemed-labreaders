Unicode true
Name "WiseMED SHIMATZU Generic Reader"
OutFile "$%OUTPUT_EXE%"
InstallDir "$PROGRAMFILES64\WiseMED SHIMATZU Generic Reader"
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
  CreateShortcut "$SMPROGRAMS\WiseMED\WiseMED SHIMATZU Generic Reader.lnk" "$INSTDIR\shimatzu-generic-reader.exe"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\install-service.ps1"'
  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'powershell -ExecutionPolicy Bypass -File "$INSTDIR\uninstall-service.ps1"'
  Delete "$SMPROGRAMS\WiseMED\WiseMED SHIMATZU Generic Reader.lnk"
  RMDir /r "$INSTDIR"
SectionEnd
