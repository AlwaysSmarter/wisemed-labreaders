@echo off
setlocal
cd /d "%~dp0"
set "PENTRA_EXE="
for %%F in ("horiba-abx-pentra400-reader.exe" "horiba-abx-pentra400-reader-v3.exe" "LastReaders_Autobio_Horiba_ABX_Pentra400.exe" "bin\horiba-abx-pentra400-reader.exe" "bin\LastReaders_Autobio_Horiba_ABX_Pentra400.exe") do if not defined PENTRA_EXE if exist "%%~F" set "PENTRA_EXE=%%~F"
if not defined PENTRA_EXE (
  echo Executabilul Pentra nu a fost gasit. Copiati acest CMD in folderul reader-ului, langa deployments.
  pause
  exit /b 1
)
if not exist "deployments\config.yaml" if not exist "deployments\config.install.yaml" (
  echo Configuratia lipseste. Copiati acest CMD in folderul reader-ului, langa deployments.
  pause
  exit /b 1
)
echo Pornire: "%PENTRA_EXE%" -config deployments/config.yaml -showlog
"%PENTRA_EXE%" -config deployments/config.yaml -showlog
set "PENTRA_EXIT=%ERRORLEVEL%"
echo.
echo Reader-ul s-a oprit cu codul %PENTRA_EXIT%.
echo Copiati ultimele mesaje afisate mai sus pentru diagnostic.
pause
exit /b %PENTRA_EXIT%
