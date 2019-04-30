#!/bin/bash

#Stop the adapter
monit stop gpsdAdapter

#Remove mtacGpioAdapter from monit
sed -i '/gpsdAdapter.pid/{N;N;N;N;d}' /etc/monitrc

#Remove the init.d script
rm /etc/init.d/gpsdAdapter

#Remove the default variables file
rm /etc/default/gpsdAdapter

#Remove the adapter log file from log rotate
rm /etc/logrotate.d/gpsdAdapter.conf

#Remove the binary
rm /usr/bin/gpsdAdapter

#restart monit
/etc/init.d/monit reload