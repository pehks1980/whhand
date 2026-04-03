#!/bin/bash 
# Exec output from pipe as command with command validation and dangerous command prevention

home_dir="/home/user/ansible"
log_file="$home_dir/exepipe.log"

# Initialize log file
[ ! -f "$log_file" ] && touch "$log_file"
[ ! -e "$home_dir/my_exe_pipe" ] && echo "Pipe file not found: $home_dir/my_exe_pipe, exiting." && exit 1

# Dangerous command blocklist
declare -a DANGEROUS_CMDS=("rm" "dd" "mkfs" "shred" "wipe" "fdisk" "parted" "shutdown" "reboot" "halt" "poweroff" "kill" "killall" ":(){:|:&")

# Allowed command whitelist (optional - use if you want strict control)
declare -a ALLOWED_CMDS=("echo" "ls" "pwd" "cat" "grep" "sed" "awk" "date" "ps" "df" "uptime" "hostname" "whoami")

dt=$(date '+%Y-%m-%d %H:%M:%S')
echo "$dt INFO: exepipe started." >> "$log_file"

validate_command() {
    local cmd="$1"
    
    # Extract the first word (the actual command)
    local cmd_name="${cmd%% *}"
    
    # Remove path to get just the executable name (e.g., /bin/rm -> rm)
    cmd_name="${cmd_name##*/}"
    
    # Check against dangerous commands
    for danger in "${DANGEROUS_CMDS[@]}"; do
        if [[ "$cmd_name" == "$danger" ]]; then
            return 1  # Command is dangerous
        fi
    done
    
    # Optional: Check against whitelist if you want strict control
    # Uncomment the section below if you want ONLY whitelisted commands
    # local allowed=0
    # for safe in "${ALLOWED_CMDS[@]}"; do
    #     if [[ "$cmd_name" == "$safe" ]]; then
    #         allowed=1
    #         break
    #     fi
    # done
    # [ $allowed -eq 0 ] && return 1
    
    return 0  # Command is safe
}

while true; do
    cmd=$(cat "$home_dir/my_exe_pipe")
    
    # Skip empty commands
    [ -z "$cmd" ] && sleep 1 && continue
    
    dt=$(date '+%Y-%m-%d %H:%M:%S')
    
    # Validate command
    if ! validate_command "$cmd"; then
        echo "$dt WARN: Blocked dangerous command: $cmd" >> "$log_file"
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
