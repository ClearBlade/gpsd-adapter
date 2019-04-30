#!/bin/bash

#Copy binary to /usr/local/bin
mv gpsdAdapter /usr/bin

#Ensure binary is executable
chmod +x /usr/bin/gpsdAdapter

#Set up init.d resources so that gpsdAdapter is started when the gateway starts
mv gpsdAdapter.etc.initd /etc/init.d/gpsdAdapter
mv gpsdAdapter.etc.default /etc/default/gpsdAdapter

#Ensure init.d script is executable
chmod +x /etc/init.d/gpsdAdapter

#Add adapter to log rotate
cat << EOF > /etc/logrotate.d/gpsdAdapter.conf
/var/log/gpsdAdapter {
    size 10M
    rotate 3
    compress
    copytruncate
    missingok
}
EOF

#Remove gpsdAdapter from monit in case it was already there
sed -i '/gpsdAdapter.pid/{N;N;N;N;d}' /etc/monitrc

#Add the adapter to monit
sed -i '/#  check process apache with pidfile/i \
  check process gpsdAdapter with pidfile \/var\/run\/gpsdAdapter.pid \
    start program = "\/etc\/init.d\/gpsdAdapter start" with timeout 60 seconds \
    stop program  = "\/etc\/init.d\/gpsdAdapter stop" \
    depends on edge \
 ' /etc/monitrc

#restart monit
/etc/init.d/monit reload

#Start the adapter
monit start gpsdAdapter

echo "gpsdAdapter Deployed"
