#!/bin/bash

CURRENT_ENV=$(env | grep -E "DISPLAY|WAYLAND|XDG|DBUS|SESSION|GTK|QT|DESKTOP|HOME|USER|SHELL|PATH|TERM|COLOR")

sudo -E dbus-launch --sh-syntax bash -c "$CURRENT_ENV hiddify"
