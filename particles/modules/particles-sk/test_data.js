export const snowfall = {
   "MaxCount": 4096,
   "Duration": 1,
   "Rate": 30,
   "Life": {
      "Input": {
         "Source": "Age",
         "TileMode": "Repeat",
         "Left": 0,
         "Right": 1
      },
      "XValues": [],
      "Segments": [
         {
            "Type": "Constant",
            "Ranged": false,
            "Bidirectional": false,
            "A0": 10
         }
      ]
   },
   "Drawable": {
      "Type": "SkCircleDrawable",
      "Radius": 1
   },
   "Spawn": [
      {
         "Type": "SkLinearVelocityAffector",
         "Enabled": true,
         "Force": false,
         "Frame": "World",
         "Angle": {
            "Input": {
               "Source": "Age",
               "TileMode": "Repeat",
               "Left": 0,
               "Right": 1
            },
            "XValues": [],
            "Segments": [
               {
                  "Type": "Constant",
                  "Ranged": true,
                  "Bidirectional": false,
                  "A0": 170,
                  "A1": 190
               }
            ]
         },
         "Strength": {
            "Input": {
               "Source": "Age",
               "TileMode": "Repeat",
               "Left": 0,
               "Right": 1
            },
            "XValues": [],
            "Segments": [
               {
                  "Type": "Constant",
                  "Ranged": true,
                  "Bidirectional": false,
                  "A0": 10,
                  "A1": 30
               }
            ]
         }
      },
      {
         "Type": "SkPositionOnPathAffector",
         "Enabled": true,
         "Input": {
            "Source": "Random",
            "TileMode": "Clamp",
            "Left": 0,
            "Right": 1
         },
         "SetHeading": false,
         "Path": "h500"
      }
   ],
   "Update": [
      {
         "Type": "SkSizeAffector",
         "Enabled": true,
         "Curve": {
            "Input": {
               "Source": "Age",
               "TileMode": "Repeat",
               "Left": 0,
               "Right": 1
            },
            "XValues": [],
            "Segments": [
               {
                  "Type": "Cubic",
                  "Ranged": true,
                  "Bidirectional": false,
                  "A0": 10,
                  "B0": 10,
                  "C0": 10,
                  "D0": 0,
                  "A1": 10,
                  "B1": 0,
                  "C1": 0,
                  "D1": 0
               }
            ]
         }
      }
   ]
};

export const spiral = {
   "MaxCount": 800,
   "Duration": 4,
   "Rate": 120,
   "Life": {
      "Input": {
         "Source": "Age",
         "TileMode": "Repeat",
         "Left": 0,
         "Right": 1
      },
      "XValues": [],
      "Segments": [
         {
            "Type": "Constant",
            "Ranged": true,
            "Bidirectional": false,
            "A0": 2,
            "A1": 3
         }
      ]
   },
   "Drawable": {
      "Type": "SkCircleDrawable",
      "Radius": 2
   },
   "Spawn": [
      {
         "Type": "SkLinearVelocityAffector",
         "Enabled": true,
         "Force": false,
         "Frame": "World",
         "Angle": {
            "Input": {
               "Source": "Age",
               "TileMode": "Repeat",
               "Left": 0,
               "Right": 1
            },
            "XValues": [],
            "Segments": [
               {
                  "Type": "Linear",
                  "Ranged": false,
                  "Bidirectional": false,
                  "A0": 0,
                  "D0": 1080
               }
            ]
         },
         "Strength": {
            "Input": {
               "Source": "Age",
               "TileMode": "Repeat",
               "Left": 0,
               "Right": 1
            },
            "XValues": [],
            "Segments": [
               {
                  "Type": "Constant",
                  "Ranged": true,
                  "Bidirectional": false,
                  "A0": 50,
                  "A1": 60
               }
            ]
         }
      }
   ],
   "Update": [
      {
         "Type": "SkSizeAffector",
         "Enabled": true,
         "Curve": {
            "Input": {
               "Source": "Age",
               "TileMode": "Repeat",
               "Left": 0,
               "Right": 1
            },
            "XValues": [],
            "Segments": [
               {
                  "Type": "Linear",
                  "Ranged": false,
                  "Bidirectional": false,
                  "A0": 0.5,
                  "D0": 2
               }
            ]
         }
      },
      {
         "Type": "SkColorAffector",
         "Enabled": true,
         "Curve": {
            "Input": {
               "Source": "Age",
               "TileMode": "Repeat",
               "Left": 0,
               "Right": 1
            },
            "XValues": [],
            "Segments": [
               {
                  "Type": "Linear",
                  "Ranged": true,
                  "A0": [ 0.0999616, 0.140218, 0.784314, 1 ],
                  "D0": [ 0.523837, 0.886396, 0.980392, 1 ],
                  "A1": [ 0.378665, 0.121107, 0.705882, 1 ],
                  "D1": [ 0.934257, 0.229599, 0.955882, 1 ]
               }
            ]
         }
      }
   ]
};