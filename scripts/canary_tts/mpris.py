#!/usr/bin/env python3
"""Minimal live MPRIS method logger for the TTS canary."""

import asyncio
import signal
import sys

from dbus_next import BusType
from dbus_next.aio import MessageBus
from dbus_next.service import PropertyAccess, ServiceInterface, method, dbus_property


class Root(ServiceInterface):
    def __init__(self):
        super().__init__("org.mpris.MediaPlayer2")

    @dbus_property(access=PropertyAccess.READ)
    def Identity(self) -> "s":
        return "Voxi MPRIS Canary"

    @dbus_property(access=PropertyAccess.READ)
    def DesktopEntry(self) -> "s":
        return "voxi-mpris-canary"

    @dbus_property(access=PropertyAccess.READ)
    def CanQuit(self) -> "b":
        return True

    @dbus_property(access=PropertyAccess.READ)
    def CanRaise(self) -> "b":
        return False

    @dbus_property(access=PropertyAccess.READ)
    def HasTrackList(self) -> "b":
        return False

    @dbus_property(access=PropertyAccess.READ)
    def SupportedUriSchemes(self) -> "as":
        return []

    @dbus_property(access=PropertyAccess.READ)
    def SupportedMimeTypes(self) -> "as":
        return []

    @method()
    def Quit(self):
        log("Quit")

    @method()
    def Raise(self):
        log("Raise")


class Player(ServiceInterface):
    def __init__(self):
        super().__init__("org.mpris.MediaPlayer2.Player")

    @dbus_property(access=PropertyAccess.READ)
    def PlaybackStatus(self) -> "s":
        return "Playing"

    @dbus_property(access=PropertyAccess.READ)
    def LoopStatus(self) -> "s":
        return "None"

    @dbus_property(access=PropertyAccess.READ)
    def Rate(self) -> "d":
        return 1.0

    @dbus_property(access=PropertyAccess.READ)
    def Shuffle(self) -> "b":
        return False

    @dbus_property(access=PropertyAccess.READ)
    def Metadata(self) -> "a{sv}":
        return {}

    @dbus_property(access=PropertyAccess.READ)
    def Volume(self) -> "d":
        return 1.0

    @dbus_property(access=PropertyAccess.READ)
    def Position(self) -> "x":
        return 0

    @dbus_property(access=PropertyAccess.READ)
    def MinimumRate(self) -> "d":
        return 1.0

    @dbus_property(access=PropertyAccess.READ)
    def MaximumRate(self) -> "d":
        return 1.0

    @dbus_property(access=PropertyAccess.READ)
    def CanGoNext(self) -> "b":
        return True

    @dbus_property(access=PropertyAccess.READ)
    def CanGoPrevious(self) -> "b":
        return True

    @dbus_property(access=PropertyAccess.READ)
    def CanPlay(self) -> "b":
        return True

    @dbus_property(access=PropertyAccess.READ)
    def CanPause(self) -> "b":
        return True

    @dbus_property(access=PropertyAccess.READ)
    def CanSeek(self) -> "b":
        return False

    @dbus_property(access=PropertyAccess.READ)
    def CanControl(self) -> "b":
        return True

    @method()
    def Next(self):
        log("Next")

    @method()
    def Previous(self):
        log("Previous")

    @method()
    def Pause(self):
        log("Pause")

    @method()
    def PlayPause(self):
        log("PlayPause")

    @method()
    def Stop(self):
        log("Stop")

    @method()
    def Play(self):
        log("Play")

    @method()
    def Seek(self, Offset: "x"):
        log(f"Seek({Offset})")

    @method()
    def SetPosition(self, TrackId: "o", Position: "x"):
        log("SetPosition")

    @method()
    def OpenUri(self, Uri: "s"):
        log("OpenUri")


def log(message):
    print(message, file=sys.stdout, flush=True)


async def main():
    bus = await MessageBus(bus_type=BusType.SESSION).connect()
    bus.export("/org/mpris/MediaPlayer2", Root())
    bus.export("/org/mpris/MediaPlayer2", Player())
    await bus.request_name("org.mpris.MediaPlayer2.VoxiCanary")
    log("READY org.mpris.MediaPlayer2.VoxiCanary")
    stopped = asyncio.Event()
    loop = asyncio.get_running_loop()
    loop.add_signal_handler(signal.SIGTERM, stopped.set)
    loop.add_signal_handler(signal.SIGINT, stopped.set)
    await stopped.wait()
    bus.disconnect()


asyncio.run(main())
