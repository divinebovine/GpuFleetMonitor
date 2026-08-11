#!/bin/bash
# This plugin checks GPU XID errors
readonly OK=0
readonly NONOK=1
readonly UNKNOWN=2

# time threshold in hours
readonly time_threshold=2
readonly kernel_log="/config/fake-kmsg.log"
readonly XID_EC="48 56 57 58 62 63 64 65 68 69 73 74 79 80 81 92 119 120"
readonly GPU_XID_TEST="GPU Xid errors detected"

if [[ ! -f $kernel_log ]]; then
	echo "$kernel_log not found. Skipping GPU Xid error test."
	exit $OK
fi

# check for any xid errors
grep -q "Xid" $kernel_log
RC=$?
if [ $RC == 0 ]; then
	for XID in $XID_EC; do
		xid_found_line=$(grep "Xid.*: $XID," $kernel_log | tail -n 1)
		if [ "$xid_found_line" != "" ]; then
			logXid=$(echo "$xid_found_line" | awk -F ',' '{print $1}')
			log_date="$(echo "$logXid" | awk '{print $1, $2, $3}') $(date +"%Y")"
			log_date=$(date -d "$log_date" +"%s")
			current_ts=$(date +"%s")
			diff=$(((current_ts - log_date) / 3600))

			if [ "$diff" -le $time_threshold ]; then
				echo "$GPU_XID_TEST: $xid_found_line. FaultCode: NHC2001"
				exit $NONOK
			else
				echo "Xid older than $time_threshold hours: $diff hours. Skipping this XID error: $logXid."
			fi
		fi
	done
  exit $OK
fi

echo "GPU XID error check passed."
exit $OK
