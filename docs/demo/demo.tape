# The README's demo. hack/demo.sh renders it to docs/demo/demo.gif in the VHS
# container, against the fake claude and the sessions setup.sh makes.

Output docs/demo/demo.gif

Set Shell bash
Set FontSize 14
Set Width 1440
Set Height 640
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms
Set WindowBar Colorful

Hide
Type "source /vhs/docs/demo/setup.sh && clear"
Enter
Sleep 1s
Show

Type "claudes ls"
Sleep 400ms
Enter
Sleep 3s

Type "claudes restart --outdated --ping"
Sleep 400ms
Enter
Sleep 4.5s

Hide
Type "clear"
Enter
Show

Type "claudes pause"
Sleep 400ms
Enter
Sleep 2.5s

Type "claudes resume --ping"
Sleep 400ms
Enter
Sleep 3.5s

Hide
Type "clear"
Enter
Show

Type "claudes session search lock race -l week"
Sleep 400ms
Enter
Sleep 3.5s

Type "claudes session ls --running"
Sleep 400ms
Enter
Sleep 2.5s

Type "claudes session ls --stopped -l today"
Sleep 400ms
Enter
Sleep 3s

Hide
Type "clear"
Enter
Show

Type "claudes session triage"
Sleep 400ms
Enter
Sleep 6s

Hide
Type "clear"
Enter
Show

Type "claudes session gc --dry-run"
Sleep 400ms
Enter
Sleep 4s
