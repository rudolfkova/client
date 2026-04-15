@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo DnD world editor — введите email и пароль в этом окне.
echo.
editor.exe
echo.
pause
