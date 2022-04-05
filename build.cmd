@echo off

:ENTRY_POINT
    setlocal EnableDelayedExpansion

    REM script base dir
    set SCRIPTDIR=%~dp0
    set PROJDIR=%SCRIPTDIR:~0,-1%

    REM Put installed packages into ./bin
    set GOBIN=%PROJDIR%\bin

    set FLAGS=

    REM Check if sources are under Git control
    if not exist ".git" goto :CHECK_BIN

    REM set BUILD=`git rev-parse --short HEAD \\ ""`
    FOR /F "tokens=*" %%X IN ('git rev-parse --short HEAD') DO (
        set BUILD=%%X
    )

    REM set BRANCH=`(git symbolic-ref --short HEAD \ tr -d \/ ) \\ ""`
    FOR /F "tokens=*" %%X IN ('git symbolic-ref --short HEAD') DO (
        set BRANCHRAW=%%X
        set BRANCH=!BRANCHRAW:/=!
    )
    if "%BRANCH%" == "main" set BRANCH=

    set FLAGS=-X github.com/matrix-org/dendrite/internal.branch=%BRANCH% -X github.com/matrix-org/dendrite/internal.build=%BUILD%

:CHECK_BIN
    if exist "bin" goto :ALL_SET
    mkdir "bin"

:ALL_SET
    set CGO_ENABLED=1
    for /D %%P in (cmd\*) do (
        go build -trimpath -ldflags "%FLAGS%" -v -o ".\bin" ".\%%P"
    )

    set CGO_ENABLED=0
    set GOOS=js
    set GOARCH=wasm
    go build -trimpath -ldflags "%FLAGS%" -o bin\main.wasm .\cmd\dendritejs-pinecone

    goto :DONE

:DONE
    echo Done
    endlocal