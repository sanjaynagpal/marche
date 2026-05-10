@echo off
setlocal
set HERE=%~dp0
if "%MARCHE_ENV%"=="" set MARCHE_ENV=dev
if "%MARCHE_CONFIG_DIR%"=="" set MARCHE_CONFIG_DIR=%HERE%..\cfg
java -Dmarche.env=%MARCHE_ENV% ^
     -Dmarche.config.dir=%MARCHE_CONFIG_DIR% ^
     -cp "%HERE%lib\*" ^
     io.sn.marche.alpha.Main
endlocal
