# Linux Autocorrect Setup (AHK Hotstring Equivalent)
# =====================================================
#
# Two scripts:
#   parse_ahk.py         — one-time conversion of your .ahk file → corrections.json
#   autocorrect_daemon.py — the persistent background daemon
#
# --------------------------------------------------------------------------
# STEP 1 — Place the scripts somewhere permanent
# --------------------------------------------------------------------------
#
#   mkdir -p ~/.local/bin/autocorrect
#   cp parse_ahk.py autocorrect_daemon.py ~/.local/bin/autocorrect/
#
# --------------------------------------------------------------------------
# STEP 2 — Convert your AHK file (one time only)
# --------------------------------------------------------------------------
#
#   python3 ~/.local/bin/autocorrect/parse_ahk.py \
#           ~/Downloads/autocorrect.ahk \
#           ~/.local/bin/autocorrect/corrections.json
#
# --------------------------------------------------------------------------
# STEP 3 — NixOS configuration.nix  (system-level)
# --------------------------------------------------------------------------

{ config, pkgs, ... }:
{
  # 1. Install required packages
  environment.systemPackages = with pkgs; [
    # Python + evdev for reading keyboard input
    (python3.withPackages (ps: [ ps.evdev ]))

    # Output tool — pick ONE based on your display server:
    xdotool   # for X11  (most common with GNOME/KDE on X)
    # wtype   # for Wayland (uncomment if you use Wayland)
    # ydotool # alternative Wayland tool (needs ydotoold service)
  ];

  # 2. Add your user to the 'input' group so evdev can read /dev/input/*
  users.users.YOUR_USERNAME.extraGroups = [ "input" ];
  #                          ^^^^^^^^^^^^^
  #                          change to your actual username
}

# --------------------------------------------------------------------------
# STEP 4 — Home-Manager systemd user service  (home.nix)
# --------------------------------------------------------------------------
# This runs the daemon automatically when you log in.
# Add this block to your home.nix:

/*
{ config, pkgs, ... }:
{
  systemd.user.services.autocorrect = {
    Unit = {
      Description = "AHK-style autocorrect daemon";
      After = [ "graphical-session.target" ];
      PartOf = [ "graphical-session.target" ];
    };

    Service = {
      Type = "simple";
      ExecStart = "${pkgs.python3.withPackages (ps: [ps.evdev])}/bin/python3 "
                + "%h/.local/bin/autocorrect/autocorrect_daemon.py "
                + "%h/.local/bin/autocorrect/corrections.json";
      Restart = "on-failure";
      RestartSec = "3s";
      # Pass display environment so xdotool/wtype knows where to type
      Environment = [
        "DISPLAY=:0"
        # "WAYLAND_DISPLAY=wayland-0"  # uncomment if using Wayland
      ];
    };

    Install.WantedBy = [ "graphical-session.target" ];
  };
}
*/

# --------------------------------------------------------------------------
# STEP 5 — Without Home-Manager: simple autostart script
# --------------------------------------------------------------------------
# Add to your ~/.xprofile (X11) or ~/.config/hypr/hyprland.conf etc.:
#
#   X11 (~/.xprofile):
#     python3 ~/.local/bin/autocorrect/autocorrect_daemon.py &
#
#   Hyprland (~/.config/hypr/hyprland.conf):
#     exec-once = python3 ~/.local/bin/autocorrect/autocorrect_daemon.py
#
#   GNOME / KDE: add it in Settings → Startup Applications
#
# --------------------------------------------------------------------------
# STEP 6 — Apply and test
# --------------------------------------------------------------------------
#
#   # Rebuild NixOS (needed for group membership + packages):
#   sudo nixos-rebuild switch
#
#   # Then log out and back in (group membership requires a new session)
#
#   # Run manually first to verify it works:
#   python3 ~/.local/bin/autocorrect/autocorrect_daemon.py --log-level DEBUG
#
#   # Type "teh " in any text box → it should correct to "the "
#
# --------------------------------------------------------------------------
# TROUBLESHOOTING
# --------------------------------------------------------------------------
#
# "No keyboard devices found"
#   → Not in input group. Run:  groups   (should show "input")
#   → If missing: sudo usermod -aG input $USER  then re-login
#   → Or verify: ls -la /dev/input/event*   (should be group-readable)
#
# Corrections fire but no text appears / backspaces appear in wrong window
#   → xdotool needs DISPLAY set. Add: export DISPLAY=:0  before running.
#   → On Wayland, switch to wtype: install wtype, set WAYLAND_DISPLAY.
#
# Lag / delay when typing
#   → Normal: evdev reads are async and very low latency.
#   → If corrections feel slow, check xdotool --delay 0 is set (it is).
#
# Only some keyboards detected
#   → Run with --log-level DEBUG to see which devices were found.
#   → Virtual/USB keyboards enumerate as separate devices; all are monitored.
#
# Want to add your own corrections
#   → Edit corrections.json directly (it's just {"wrong": "right"} pairs)
#   → Or re-run parse_ahk.py after editing your .ahk file
#
# --------------------------------------------------------------------------
# WAYLAND NOTES
# --------------------------------------------------------------------------
# wtype is simpler and recommended. Install with:
#   environment.systemPackages = with pkgs; [ wtype python3Packages.evdev ];
#
# ydotool works too but needs a background daemon:
#   services.ydotool.enable = true;  # in configuration.nix
#   users.users.YOUR_USERNAME.extraGroups = [ "input" "ydotool" ];
