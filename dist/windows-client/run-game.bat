@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo DnD game — введите gateway, email и пароль в этом окне.
echo.
game.exe
echo.
pause
