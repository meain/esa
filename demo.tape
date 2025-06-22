# ESA Demo

Output demo.gif

Require esa

Set Shell "zsh"
Set FontSize 14
Set Width 1200
Set Height 800
Set PlaybackSpeed 1.0
Set TypingSpeed 65ms
Set Theme "Alabaster"
Set FontFamily "Monoflow"

# Show the intro
Type "ESA | github.com/meain/esa"
Sleep 1s
Backspace 34

# Basic usage with default agent
Type "esa what is the current price of mangos in India"
Enter
Sleep 6s

# 2. Continue conversation using -c flag
Type "esa -c show a notification to user about it"
Enter
Sleep 5s

# Show commands
Type "esa --show-commands what is 10 + 3 multiplied by 7"
Enter
Sleep 4s

# 4. Show agent details
Type "esa --show-agent +coder"
Sleep 3s
Enter
Sleep 4s

# 5. REPL mode demonstration
Type "esa --repl"
Enter
Sleep 2s

# Show config
Type "/config"
Enter
Sleep 2s

# Switch to web agent
Type "/agent +web"
Enter
Sleep 2s

# Ask a question
Type "what are the benefits of eating mangoes?"
Enter
Sleep 6s

# Switch model
Type "/agent +notools"
Enter
Type "/model mercury"
Enter
Sleep 1s

# Ask follow-up question
Type "how many r in strawberry?"
Enter
Sleep 3s

# Exit REPL
Type "/exit"
Enter
Sleep 2s

Type "ESA | github.com/meain/esa"
Sleep 3s