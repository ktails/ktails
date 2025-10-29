# VHS tape file for ktails demo
# Enhanced version with better pacing and visual polish

Output ktails.gif

Require echo

# Settings - must come before any non-setting commands
Set Shell "bash"
Set FontSize 14
Set Width 1210
Set Height 600
Set Padding 20
Set BorderRadius 10
Set WindowBar Colorful
Set TypingSpeed 50ms
Set PlaybackSpeed 1.0
Set Theme "Catppuccin Mocha"

# Optional: Add margin for a more polished look
# Set Margin 10
# Set MarginFill "#1e1e2e"

# Hide initial setup
Hide
Type "clear"
Enter
Show

# Welcome message with better timing
Type "echo 'Welcome to ktails!'"
Sleep 500ms
Enter
Sleep 1.5s

# Launch the application
Type "./build/ktails"
Sleep 800ms
Enter

# Give the app time to fully render
Sleep 2.5s

# Navigate down the list
Down
Sleep 500ms
Down
Sleep 500ms

# Select an item
Space
Sleep 1.5s

# Navigate back up
Up
Sleep 500ms

# Select another item
Space
Sleep 1.5s

# Switch tabs
Ctrl+T
Sleep 1s
Tab
Sleep 1.5s
Tab
Sleep 1.5s

# Navigate back
Shift+Tab
Sleep 800ms

# Move left in the interface
Ctrl+T
Sleep 500ms
Space 1 
Enter
Sleep 500ms

# Pause to show final state
Sleep 3s

# Take a screenshot before quitting (optional)
# Screenshot ktails_screenshot.png

# Quit the application
Type "q"
Sleep 1s