RequestExecutionLevel admin
ShowInstDetails show
ShowUninstDetails show

!include "include/common.nsh"

!ifndef APP_NAME
  !error "APP_NAME is required"
!endif
!ifndef APP_VERSION
  !error "APP_VERSION is required"
!endif
!ifndef APP_BINARY_NAME
  !error "APP_BINARY_NAME is required"
!endif
!ifndef APP_INSTALL_DIR_NAME
  !error "APP_INSTALL_DIR_NAME is required"
!endif
!ifndef APP_PAYLOAD_DIR
  !error "APP_PAYLOAD_DIR is required"
!endif
!ifndef OUTPUT_EXE
  !error "OUTPUT_EXE is required"
!endif

!ifdef APP_ICON
  Icon "${APP_ICON}"
  UninstallIcon "${APP_ICON}"
!endif

Name "${APP_NAME}"
Caption "${APP_NAME} ${APP_VERSION} Setup"
OutFile "${OUTPUT_EXE}"
InstallDir "$PROGRAMFILES64\${APP_INSTALL_DIR_NAME}"
InstallDirRegKey HKLM "Software\WiseMED\${APP_INSTALL_DIR_NAME}" "InstallDir"

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetShellVarContext all
  SetOutPath "$INSTDIR"
  File /r "${APP_PAYLOAD_DIR}\*"

  WriteRegStr HKLM "Software\WiseMED\${APP_INSTALL_DIR_NAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_INSTALL_DIR_NAME}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_INSTALL_DIR_NAME}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_INSTALL_DIR_NAME}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_INSTALL_DIR_NAME}" "UninstallString" "$INSTDIR\Uninstall.exe"

  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  !insertmacro WiseMEDShortcut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_BINARY_NAME}"
  !insertmacro WiseMEDShortcut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_BINARY_NAME}"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Uninstall"
  SetShellVarContext all
  Delete "$DESKTOP\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"
  Delete "$INSTDIR\${APP_BINARY_NAME}"
  Delete "$INSTDIR\Uninstall.exe"
  Delete "$INSTDIR\deployments\config.install.yaml"
  RMDir /r "$INSTDIR\deployments\help"
  RMDir "$INSTDIR\deployments"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_INSTALL_DIR_NAME}"
  DeleteRegKey HKLM "Software\WiseMED\${APP_INSTALL_DIR_NAME}"
SectionEnd

Function .onInstSuccess
  MessageBox MB_YESNO "Instalarea s-a terminat cu succes.$\r$\nPornesc acum ${APP_NAME}?" IDNO done
  Exec '"$INSTDIR\${APP_BINARY_NAME}"'
done:
FunctionEnd
