{ ... }: {
  # home.packages = with pkgs; [ opentabletdriver ];

  home.file.".config/OpenTabletDriver/Plugins/" = {
    source = ./Plugins;
    recursive = true;
  };
  home.file.".config/OpenTabletDriver/settings.json".text = ''
    {
      "Profiles": [
        {
          "Tablet": "Wacom CTL-672",
          "OutputMode": {
            "Path": "OpenTabletDriver.Desktop.Output.AbsoluteMode",
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
              "Enable": false
            },
            {
              "Path": "RadialFollow.RadialFollowSmoothingTabletSpace",
              "Settings": [
                {
                  "Property": "OuterRadius",
                  "Value": 0.633216
                },
                {
                  "Property": "InnerRadius",
                  "Value": 0.271
                },
                {
                  "Property": "SmoothingCoefficient",
                  "Value": 0.271
                },
                {
                  "Property": "SoftKneeScale",
                  "Value": 0.543
                },
                {
                  "Property": "SmoothingLeakCoefficient",
                  "Value": 0.181
                }
              ],
              "Enable": true
            },
            {
              "Path": "SpringInterpolator.SpringInterpolator",
              "Settings": [
                {
                  "Property": "Stiffness",
                  "Value": 1.5
                },
                {
                  "Property": "Damping",
                  "Value": 3.0
                },
                {
                  "Property": "StepSize",
                  "Value": 40.0
                },
                {
                  "Property": "Frequency",
                  "Value": 1000.0
                }
              ],
              "Enable": false
            },
            {
              "Path": "TemporalResampler",
              "Settings": [
                {
                  "Property": "followRadius",
                  "Value": 0.0
                },
                {
                  "Property": "latency",
                  "Value": 0.0
                },
                {
                  "Property": "reverseSmoothing",
                  "Value": 1.0
                },
                {
                  "Property": "extraFrames",
                  "Value": true
                },
                {
                  "Property": "loggingEnabled",
                  "Value": false
                },
                {
                  "Property": "Frequency",
                  "Value": 1000.0
                },
                {
                  "Property": "frameShift",
                  "Value": 0.0
                }
              ],
              "Enable": true
            },
            {
              "Path": "Kuuube_s_Chatter_Exterminator.Kuuube_s_CHATTER_EXTERMINATOR_SMOOTH",
              "Settings": [
                {
                  "Property": "Chatter_Extermination_Strength",
                  "Value": 6.0
                }
              ],
              "Enable": false
            },
            {
              "Path": "RadialFollow.RadialFollowSmoothingScreenSpace",
              "Settings": [
                {
                  "Property": "OuterRadius",
                  "Value": 5.0
                },
                {
                  "Property": "InnerRadius",
                  "Value": 0.0
                },
                {
                  "Property": "SmoothingCoefficient",
                  "Value": 0.95
                },
                {
                  "Property": "SoftKneeScale",
                  "Value": 1.0
                },
                {
                  "Property": "SmoothingLeakCoefficient",
                  "Value": 0.0
                }
              ],
              "Enable": false
            },
            {
              "Path": "TabletDriverFilters.Hawku.Smoothing",
              "Settings": [
                {
                  "Property": "Latency",
                  "Value": 2.0
                },
                {
                  "Property": "Frequency",
                  "Value": 1000.0
                }
              ],
              "Enable": false
            },
            {
              "Path": "Kuuube_s_Chatter_Exterminator.Kuuube_s_CHATTER_EXTERMINATOR_RAW",
              "Settings": [
                {
                  "Property": "Chatter_Extermination_Strength",
                  "Value": 2.0
                }
              ],
              "Enable": false
            },
            {
              "Path": "OpenTabletDriver.Plugin.NoiseReduction",
              "Settings": [
                {
                  "Property": "Samples",
                  "Value": 10
                },
                {
                  "Property": "DistanceThreshold",
                  "Value": 0.5
                }
              ],
              "Enable": false
            }
          ],
          "AbsoluteModeSettings": {
            "Display": {
              "Width": 2560.0,
              "Height": 1440.0,
              "X": 3328.0,
              "Y": 710.0,
              "Rotation": 0.0
            },
            "Tablet": {
              "Width": 52.0,
              "Height": 40.0,
              "X": 128.42763,
              "Y": 66.775406,
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
            "PenButtons": [
              null,
              null
            ],
            "AuxButtons": [],
            "MouseButtons": [],
            "MouseScrollUp": null,
            "MouseScrollDown": null
          }
        }
      ],
      "LockUsableAreaDisplay": false,
      "LockUsableAreaTablet": false,
      "Tools": []
    }

  '';
}
