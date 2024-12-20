{ config, pkgs, ... }:

{
  nixpkgs.overlays = [
    (final: prev: {
      opentabletdriver = prev.opentabletdriver.overrideAttrs (oldAttrs: {
        version = "0.6.4.0";
        src = final.fetchFromGitHub {
          owner = "OpenTabletDriver";
          repo = "OpenTabletDriver";
          rev = "v0.6.4.0";
          sha256 = "sha256-Hs6Zg0QLpP0Zw5eDqkGGXWfgHhfZpRQBvFZbPDrNxcw=";
        };
      });
    })
  ];

  home.packages = with pkgs; [
    opentabletdriver
  ];

  home.file.".config/OpenTabletDriver/settings.json".text = ''
{
  "Profiles": [
    {
      "Tablet": "Wacom CTL-672",
      "OutputMode": {
        "Path": "OpenTabletDriver.Desktop.Output.LinuxArtistMode",
        "Settings": [],
        "Enable": true
      },
      "Filters": [
        {
          "Path": "TabletDriverFilters.Devocub.Antichatter",
          "Settings": [
            {
              "Property": "AntichatterOffsetY",
              "Value": 1.0
            },
            {
              "Property": "PredictionEnabled",
              "Value": null
            },
            {
              "Property": "PredictionStrength",
              "Value": 1.1
            },
            {
              "Property": "PredictionSharpness",
              "Value": 1.0
            },
            {
              "Property": "PredictionOffsetX",
              "Value": 3.0
            },
            {
              "Property": "PredictionOffsetY",
              "Value": 0.3
            },
            {
              "Property": "Frequency",
              "Value": 1000.0
            },
            {
              "Property": "AntichatterStrength",
              "Value": 3.0
            },
            {
              "Property": "AntichatterMultiplier",
              "Value": 100.0
            },
            {
              "Property": "AntichatterOffsetX",
              "Value": 1.5
            },
            {
              "Property": "Latency",
              "Value": 10.0
            }
          ],
          "Enable": true
        }
      ],
      "AbsoluteModeSettings": {
        "Display": {
          "Width": 2560.0,
          "Height": 1440.0,
          "X": 1280.0,
          "Y": 720.0,
          "Rotation": 0.0
        },
        "Tablet": {
          "Width": 58.0,
          "Height": 45.0,
          "X": 29.0,
          "Y": 62.483784,
          "Rotation": 0.0
        },
        "EnableClipping": true,
        "EnableAreaLimiting": false,
        "LockAspectRatio": false
      },
      "RelativeModeSettings": {
        "XSensitivity": 10.0,
        "YSensitivity": 10.0,
        "RelativeRotation": 0.0,
        "RelativeResetDelay": "00:00:00.1000000"
      },
      "Bindings": {
        "TipActivationThreshold": 0.0,
        "TipButton": {
          "Path": "OpenTabletDriver.Desktop.Binding.MouseBinding",
          "Settings": [
            {
              "Property": "Button",
              "Value": "Left"
            }
          ],
          "Enable": true
        },
        "EraserActivationThreshold": 0.0,
        "EraserButton": null,
        "PenButtons": []
      }
    }
  ]
}
  '';
}
