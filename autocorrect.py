#!/usr/bin/env python3
"""
Linux autocorrect daemon — AHK hotstring equivalent.

Reads keyboard input via evdev (works on both X11 and Wayland).
Sends corrections via xdotool (X11) or wtype/ydotool (Wayland).

Requirements:
  pip install evdev          (or nixpkgs: python3Packages.evdev)
  apt/nix: xdotool           (X11)   OR   wtype + ydotool   (Wayland)

Your user must be in the 'input' group:
  NixOS:  users.users.<name>.extraGroups = [ "input" ];
  Other:  sudo usermod -aG input $USER  (then re-login)
"""

import asyncio
from syslog import LOG_WARNING
import evdev
import json
import logging
import os
import subprocess
import sys
from pathlib import Path
from evdev import InputDevice, ecodes, UInput

# ---------------------------------------------------------------------------
# Key → character tables
# ---------------------------------------------------------------------------

NORMAL: dict[int, str] = {
  ecodes.KEY_A: "a",
  ecodes.KEY_B: "b",
  ecodes.KEY_C: "c",
  ecodes.KEY_D: "d",
  ecodes.KEY_E: "e",
  ecodes.KEY_F: "f",
  ecodes.KEY_G: "g",
  ecodes.KEY_H: "h",
  ecodes.KEY_I: "i",
  ecodes.KEY_J: "j",
  ecodes.KEY_K: "k",
  ecodes.KEY_L: "l",
  ecodes.KEY_M: "m",
  ecodes.KEY_N: "n",
  ecodes.KEY_O: "o",
  ecodes.KEY_P: "p",
  ecodes.KEY_Q: "q",
  ecodes.KEY_R: "r",
  ecodes.KEY_S: "s",
  ecodes.KEY_T: "t",
  ecodes.KEY_U: "u",
  ecodes.KEY_V: "v",
  ecodes.KEY_W: "w",
  ecodes.KEY_X: "x",
  ecodes.KEY_Y: "y",
  ecodes.KEY_Z: "z",
  ecodes.KEY_0: "0",
  ecodes.KEY_1: "1",
  ecodes.KEY_2: "2",
  ecodes.KEY_3: "3",
  ecodes.KEY_4: "4",
  ecodes.KEY_5: "5",
  ecodes.KEY_6: "6",
  ecodes.KEY_7: "7",
  ecodes.KEY_8: "8",
  ecodes.KEY_9: "9",
  ecodes.KEY_SPACE: " ",
  ecodes.KEY_MINUS: "-",
  ecodes.KEY_EQUAL: "=",
  ecodes.KEY_LEFTBRACE: "[",
  ecodes.KEY_RIGHTBRACE: "]",
  ecodes.KEY_SEMICOLON: ";",
  ecodes.KEY_APOSTROPHE: "'",
  ecodes.KEY_GRAVE: "`",
  ecodes.KEY_BACKSLASH: "\\",
  ecodes.KEY_COMMA: ",",
  ecodes.KEY_DOT: ".",
  ecodes.KEY_SLASH: "/",
  ecodes.KEY_ENTER: "\n",
  ecodes.KEY_KPENTER: "\n",
}

SHIFTED: dict[int, str] = {
  ecodes.KEY_A: "A",
  ecodes.KEY_B: "B",
  ecodes.KEY_C: "C",
  ecodes.KEY_D: "D",
  ecodes.KEY_E: "E",
  ecodes.KEY_F: "F",
  ecodes.KEY_G: "G",
  ecodes.KEY_H: "H",
  ecodes.KEY_I: "I",
  ecodes.KEY_J: "J",
  ecodes.KEY_K: "K",
  ecodes.KEY_L: "L",
  ecodes.KEY_M: "M",
  ecodes.KEY_N: "N",
  ecodes.KEY_O: "O",
  ecodes.KEY_P: "P",
  ecodes.KEY_Q: "Q",
  ecodes.KEY_R: "R",
  ecodes.KEY_S: "S",
  ecodes.KEY_T: "T",
  ecodes.KEY_U: "U",
  ecodes.KEY_V: "V",
  ecodes.KEY_W: "W",
  ecodes.KEY_X: "X",
  ecodes.KEY_Y: "Y",
  ecodes.KEY_Z: "Z",
  ecodes.KEY_0: ")",
  ecodes.KEY_1: "!",
  ecodes.KEY_2: "@",
  ecodes.KEY_3: "#",
  ecodes.KEY_4: "$",
  ecodes.KEY_5: "%",
  ecodes.KEY_6: "^",
  ecodes.KEY_7: "&",
  ecodes.KEY_8: "*",
  ecodes.KEY_9: "(",
  ecodes.KEY_SPACE: " ",
  ecodes.KEY_MINUS: "_",
  ecodes.KEY_EQUAL: "+",
  ecodes.KEY_LEFTBRACE: "{",
  ecodes.KEY_RIGHTBRACE: "}",
  ecodes.KEY_SEMICOLON: ":",
  ecodes.KEY_APOSTROPHE: '"',
  ecodes.KEY_GRAVE: "~",
  ecodes.KEY_BACKSLASH: "|",
  ecodes.KEY_COMMA: "<",
  ecodes.KEY_DOT: ">",
  ecodes.KEY_SLASH: "?",
  ecodes.KEY_ENTER: "\n",
  ecodes.KEY_KPENTER: "\n",
}

# Keys that signal the user has moved the cursor — reset the buffer
RESET_KEYS = {
  ecodes.KEY_LEFT,
  ecodes.KEY_RIGHT,
  ecodes.KEY_UP,
  ecodes.KEY_DOWN,
  ecodes.KEY_HOME,
  ecodes.KEY_END,
  ecodes.KEY_PAGEUP,
  ecodes.KEY_PAGEDOWN,
  ecodes.KEY_DELETE,
  ecodes.KEY_ESC,
}

# Characters that AHK treats as hotstring end-triggers
# (the char that follows the misspelling to fire the correction)
TRIGGER_CHARS = set(" \t\n-()[]{}';:/\\,.?!@#$%^&*+=<>|`~\"")


# ---------------------------------------------------------------------------
# Display / output backend detection
# ---------------------------------------------------------------------------


def _cmd_exists(cmd: str) -> bool:
  from shutil import which

  return which(cmd) is not None


def detect_backend() -> str:
  """Return 'wayland' or 'x11' based on the running session."""
  if os.environ.get("WAYLAND_DISPLAY"):
    return "wayland"
  if os.environ.get("DISPLAY"):
    return "x11"
  # Fallback: try to guess
  return "wayland" if _cmd_exists("wtype") else "x11"


def send_correction(n_backspaces: int, text: str, backend: str) -> None:
  """Delete n_backspaces chars then type text."""
  try:
    if backend == "wayland":
      # wtype handles both keys and text in one invocation — no daemon needed.
      # -k BackSpace sends a backspace key; positional args type literal text.
      # Build: wtype -k BackSpace -k BackSpace ... "replacement text"
      cmd = ["wtype"]
      for _ in range(n_backspaces):
        cmd += ["-k", "BackSpace"]
      if text:
        cmd.append(text)
      _ = subprocess.run(cmd, check=True, timeout=2)
    else: # x11
      if n_backspaces:
        _ = subprocess.run(
          ["xdotool", "key", "--clearmodifiers"]
          + ["BackSpace"] * n_backspaces,
          check=True,
          timeout=2,
        )
      if text:
        _ = subprocess.run(
          ["xdotool", "type", "--clearmodifiers", "--delay", "0", text],
          check=True,
          timeout=2,
        )
  except (subprocess.CalledProcessError, FileNotFoundError) as e:
    logging.warning("send_correction failed: %s", e)


# ---------------------------------------------------------------------------
# Keyboard device discovery
# ---------------------------------------------------------------------------


def find_keyboards() -> list[InputDevice]:
  """Return all devices that look like keyboards."""
  keyboards = []
  for path in evdev.list_devices():
    try:
      dev = InputDevice(path)
      caps = dev.capabilities()
      keys = caps.get(ecodes.EV_KEY, [])
      # Must have letter keys to be considered a keyboard
      if ecodes.KEY_A in keys and ecodes.KEY_Z in keys:
        keyboards.append(dev)
        logging.info("Found keyboard: %s (%s)", dev.name, path)
    except (PermissionError, OSError) as e:
      logging.debug("Skipping %s: %s", path, e)
  return keyboards


# ---------------------------------------------------------------------------
# Core autocorrect state machine
# ---------------------------------------------------------------------------


class AutoCorrect:
  BUFFER_MAX = 150

  def __init__(self, corrections: dict[str, str]):
    self.corrections = dict(sorted(corrections.items(), key=lambda kv: -len(kv[0])))
    self.buffer: str = ""
    self.shift_held = False
    self.capslock_on = False

  def handle_event(self, event, ui: UInput):
    """
    Processes event and returns True if the key was 'swallowed' or modified.
    Otherwise returns False so the caller can pass the original event through.
    """
    if event.type != ecodes.EV_KEY:
      return False

    key = event.code
    action = event.value # 0=up, 1=down, 2=hold

    # Modifier Tracking
    if key in (ecodes.KEY_LEFTSHIFT, ecodes.KEY_RIGHTSHIFT):
      self.shift_held = (action != 0)
      return False

    if key == ecodes.KEY_CAPSLOCK and action == 1:
      self.capslock_on = not self.capslock_on
      return False

    if action == 0: # Key Up: always pass through to avoid stuck keys
      return False

    # Reset logic
    if key in RESET_KEYS:
      self.buffer = ""
      return False

    if key == ecodes.KEY_BACKSPACE:
      self.buffer = self.buffer[:-1]
      return False

    # Character Mapping
    is_shifted = self.shift_held ^ self.capslock_on
    table = SHIFTED if is_shifted else NORMAL
    char = table.get(key)

    if char is None:
      return False

    self.buffer += char
    if len(self.buffer) > self.BUFFER_MAX:
      self.buffer = self.buffer[-self.BUFFER_MAX:]

    # Trigger Logic

    if char in TRIGGER_CHARS or key in (ecodes.KEY_ENTER, ecodes.KEY_KPENTER):
      actual_char = "\n" if key in (ecodes.KEY_ENTER, ecodes.KEY_KPENTER) else char
      print(self.buffer)
      # Check for match
      typed_before = self.buffer[:-1]
      for wrong, right in self.corrections.items():
        if typed_before.endswith(wrong):
          self.apply_correction(ui, wrong, right, key)
          # Update internal buffer
          self.buffer=''
          # self.buffer = self.buffer[:-(len(wrong)+1)] + right + actual_char
          return True # Swallow the trigger; apply_correction handles it

    return False

  def apply_correction(self, ui: UInput, wrong: str, right: str, trigger_key: int):
    """Deletes the wrong word and sends the right word + the trigger."""
    # 1. Send Backspaces for the 'wrong' word
    for _ in range(len(wrong)):
      ui.write(ecodes.EV_KEY, ecodes.KEY_BACKSPACE, 1)
      ui.write(ecodes.EV_KEY, ecodes.KEY_BACKSPACE, 0)

    # 2. Type the 'right' word
    # Note: For a robust version, you'd map 'right' chars back to keycodes.
    # Simple hack: use the UInput.type() method if available or map manually.
    print(right)
    for c in right:
      self.type_char(ui, c)

    # 3. Finally, send the original trigger key (Space, Enter, etc.)
    ui.write(ecodes.EV_KEY, trigger_key, 1)
    ui.write(ecodes.EV_KEY, trigger_key, 0)
    ui.syn()

  def type_char(self, ui, char):
    """Basic char-to-keycode injector for the virtual keyboard."""
    # Reverse lookup from our tables
    source_table = SHIFTED if char.isupper() or char in SHIFTED.values() else NORMAL
    for code, c in source_table.items():
      if c == char:
        if source_table == SHIFTED:
          ui.write(ecodes.EV_KEY, ecodes.KEY_LEFTSHIFT, 1)
        ui.write(ecodes.EV_KEY, code, 1)
        ui.write(ecodes.EV_KEY, code, 0)
        if source_table == SHIFTED:
          ui.write(ecodes.EV_KEY, ecodes.KEY_LEFTSHIFT, 0)
        return

async def monitor_device(dev: InputDevice, ac: AutoCorrect):
  # Create virtual device
  ui = UInput.from_device(dev, name="Autocorrect-Virtual")

  # IMPORTANT: Grab device to block raw input
  dev.grab()
  logging.info(f"Blocking raw input from: {dev.name}")

  try:
    async for event in dev.async_read_loop():
      swallowed = ac.handle_event(event, ui)
      if not swallowed:
        ui.write_event(event)
        ui.syn()
  finally:
    dev.ungrab()


async def main_loop(corrections_path: str) -> None:
  # Load corrections
  with open(corrections_path, encoding="utf-8") as f:
    corrections = json.load(f)
  logging.info("Loaded %d corrections from %s", len(corrections), corrections_path)

  keyboards = find_keyboards()
  ac = AutoCorrect(corrections)
  tasks = []

  for dev in keyboards:
    # 1. Skip virtual devices to prevent feedback loops
    name_lower = dev.name.lower()
    if any(x in name_lower for x in ["virtual", "ydotool", "keyd", "autocorrect"]):
      continue

    try:
      # 2. Test the grab immediately.
      # If keyd or another daemon has it, this will trigger the OSError.
      dev.grab()
      # If we got here, we successfully grabbed it.
      # We ungrab briefly so the async monitor can manage it.
      dev.ungrab()

      logging.info(f"Adding task for: {dev.name}")
      tasks.append(monitor_device(dev, ac))

    except OSError as e:
      if e.errno == 16:
        logging.warning(f"Skipping {dev.name}: Device busy (likely grabbed by keyd)")
      else:
        logging.error(f"Failed to access {dev.name}: {e}")

  if not tasks:
    logging.error("No accessible physical keyboards found.")
    return

  await asyncio.gather(*tasks)


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
  import argparse

  parser = argparse.ArgumentParser(
    description="AHK-style autocorrect daemon for Linux"
  )
  os.makedirs(
    os.path.join(
      os.environ.get("XDG_CONFIG_HOME", os.path.expanduser("~/.config")),
      "autocorrect_daemon",
    ),
    exist_ok=True,
  )
  _ = parser.add_argument(
    "corrections",
    nargs="?",
    default=os.path.join(
      os.environ.get("XDG_CONFIG_HOME", os.path.expanduser("~/.config")),
      "autocorrect_daemon",
      "corrections.json",
    ),
    help="Path to corrections JSON file (default: corrections.json next to this script)",
  )
  _ = parser.add_argument(
    "--log-level",
    default="INFO",
    choices=["DEBUG", "INFO", "WARNING", "ERROR"],
  )
  args = parser.parse_args()

  logging.basicConfig(
    level=getattr(logging, args.log_level),
    format="%(asctime)s %(levelname)s %(message)s",
    datefmt="%H:%M:%S",
  )

  try:
    asyncio.run(main_loop(args.corrections))
  except KeyboardInterrupt:
    logging.info("Stopped.")
