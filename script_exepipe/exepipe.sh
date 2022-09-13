#!/bin/bash 
# exec output from pipe as command in infinite cycle; there is only one security - all cmds are run under user

home_dir=/home/user/ansible

dt=$(date '+%d/%m/%Y %H:%M:%S');

#touch -a $home_dir/exepip.log

echo "$dt exepipe started." >> $home_dir/exepipe.log

while true 
do

     cmd=$(cat $home_dir/my_exe_pipe);

     dt=$(date '+%d/%m/%Y %H:%M:%S');

     echo "$dt received command: $cmd " >> $home_dir/exepipe.log	
     
     eval $cmd &>> $home_dir/exepipe.log

     dt=$(date '+%d/%m/%Y %H:%M:%S');

     echo "$dt command finished " >> $home_dir/exepipe.log
done

