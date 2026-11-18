package shell

// posixInit wraps claudes in a shell function, because only the shell itself
// can change its directory. Every other command goes to the claudes program.
const posixInit = `# claudes shell integration: claudes cd NAME changes to a session's directory.
claudes() {
  if [ "$1" = cd ]; then
    shift
    local dir
    dir="$(command claudes session where --one -o text -- "$@")" && cd -- "$dir"
  else
    command claudes "$@"
  fi
}
`

const pwshInit = `# claudes shell integration: claudes cd NAME changes to a session's directory.
function claudes {
  $exe = Get-Command claudes -CommandType Application -ErrorAction Stop | Select-Object -First 1
  if ($args.Count -gt 0 -and $args[0] -eq 'cd') {
    $rest = @($args | Select-Object -Skip 1)
    # claudes prints UTF-8, which PowerShell reads in the console's code page.
    $encoding = [Console]::OutputEncoding
    [Console]::OutputEncoding = [Text.Encoding]::UTF8
    try { $dir = & $exe session where --one -o text '--' @rest }
    finally { [Console]::OutputEncoding = $encoding }
    if ($LASTEXITCODE -eq 0 -and $dir) { Set-Location -LiteralPath $dir }
  } else {
    & $exe @args
  }
}
`

// fishInit mirrors posixInit. Fish sets $status after a command substitution,
// and string collect keeps a directory with a trailing newline or spaces whole.
const fishInit = `# claudes shell integration: claudes cd NAME changes to a session's directory.
function claudes --wraps claudes --description 'claudes, with claudes cd NAME'
    if test (count $argv) -gt 0; and test "$argv[1]" = cd
        set -l dir (command claudes session where --one -o text -- $argv[2..-1] | string collect)
        and cd -- $dir
    else
        command claudes $argv
    end
end
`

var inits = map[string]string{"bash": posixInit, "zsh": posixInit, "fish": fishInit, "pwsh": pwshInit, "powershell": pwshInit}

// Init is the integration for a shell, such as bash: a function that makes
// claudes cd change the shell's directory. ok is false for a shell without
// one.
func Init(name string) (script string, ok bool) {
	script, ok = inits[name]
	return script, ok
}
