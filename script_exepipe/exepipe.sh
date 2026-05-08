#!/bin/bash 
# Exec output from pipe as command with command validation and dangerous command prevention

home_dir="/home/user/ansible"
log_file="$home_dir/exepipe.log"

# Initialize log file
[ ! -f "$log_file" ] && touch "$log_file"
[ ! -e "$home_dir/my_exe_pipe" ] && echo "Pipe file not found: $home_dir/my_exe_pipe, exiting." && exit 1

# Allowed command whitelist. Override with:
# ALLOWED_CMDS="ansible-playbook /usr/bin/ansible-playbook" ./exepipe.sh
declare -a DEFAULT_ALLOWED_CMDS=("ansible-playbook")
if [[ -n "${ALLOWED_CMDS:-}" ]]; then
    read -r -a ALLOWED_CMD_LIST <<< "$ALLOWED_CMDS"
else
    declare -a ALLOWED_CMD_LIST=("${DEFAULT_ALLOWED_CMDS[@]}")
fi

dt=$(date '+%Y-%m-%d %H:%M:%S')
echo "$dt INFO: exepipe started." >> "$log_file"

validate_command() {
    local cmd="$1"

    case "$cmd" in
        *";"*|*"&"*|*"|"*|*"<"*|*">"*|*'$('*|*'`'*|*'$'*)
            return 1
            ;;
    esac

    # Extract the first word (the actual command)
    local cmd_name="${cmd%% *}"

    # Remove path to get just the executable name (e.g., /bin/rm -> rm)
    cmd_name="${cmd_name##*/}"

    for safe in "${ALLOWED_CMD_LIST[@]}"; do
        if [[ "$cmd_name" == "$safe" || "${cmd%% *}" == "$safe" ]]; then
            return 0
        fi
    done

    return 1
}

while true; do
    if ! IFS= read -r cmd < "$home_dir/my_exe_pipe"; then
        sleep 1
        continue
    fi
    
    # Skip empty commands
    [ -z "$cmd" ] && sleep 1 && continue
    
    dt=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Validate command
    if ! validate_command "$cmd"; then
        echo "$dt WARN: Blocked command: $cmd" >> "$log_file"
        sleep 1
        continue
    fi
    
    # Log and execute command
    echo "$dt INFO: Received command: $cmd" >> "$log_file"
    bash -c "$cmd" &>> "$log_file"
    exit_code=$?
    
    dt=$(date '+%Y-%m-%d %H:%M:%S')
    echo "$dt INFO: Command finished with exit code: $exit_code" >> "$log_file"
    sleep 1
done
