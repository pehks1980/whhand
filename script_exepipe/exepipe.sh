#!/bin/bash 
# exec output from pipe as command in infinite cycle; there is only one security - all cmds are run under user

home_dir=/home/user/ansible

dt=$(date '+%d/%m/%Y %H:%M:%S');

echo "$dt started." >> $home_dir/exepipe.log

while true 
do
     dt=$(date '+%d/%m/%Y %H:%M:%S');

     cmd=$(cat $home_dir/my_exe_pipe);

     echo "$dt received command: $cmd " >> $home_dir/exepipe.log	
     
     eval $cmd &>> $home_dir/exepipe.log

     dt=$(date '+%d/%m/%Y %H:%M:%S');

     echo "$dt command finished " >> $home_dir/exepipe.log
done

