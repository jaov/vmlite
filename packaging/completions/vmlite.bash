# bash completion for vmlite
_vmlite() {
    local cur prev words cword
    if declare -F _init_completion >/dev/null 2>&1; then
        _init_completion -n = || return
    else
        COMPREPLY=()
        cur="${COMP_WORDS[COMP_CWORD]}"
        prev="${COMP_WORDS[COMP_CWORD-1]}"
        words=("${COMP_WORDS[@]}")
        cword=$COMP_CWORD
    fi

    local commands="up provision status start stop restart ssh scp console list destroy validate schema install doctor version help"
    local global_opts="--help -h --version"

    if [[ $cword -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "$commands $global_opts" -- "$cur") )
        compopt -o default 2>/dev/null
        return
    fi

    local opts=""
    case "${words[1]}" in
        up)
            opts="--connect --state-dir --recreate --dry-run --offline --help"
            ;;
        provision)
            opts="--state-dir --force --dry-run --timeout --help"
            ;;
        status|start|stop|restart|destroy|list)
            opts="--state-dir --help"
            ;;
        ssh|scp|console)
            opts="--state-dir --user --help"
            ;;
        validate)
            opts="--state-dir --help"
            ;;
        install)
            opts="--prefix --binary --binary-url --binary-sha256 --no-deps --dry-run --help"
            ;;
        doctor)
            opts="--prefix --help"
            ;;
        version|schema)
            opts="--help"
            ;;
    esac

    if [[ $cur == -* ]]; then
        COMPREPLY=( $(compgen -W "$opts" -- "$cur") )
        return
    fi

    COMPREPLY=( $(compgen -f -- "$cur") )
}

complete -F _vmlite vmlite
