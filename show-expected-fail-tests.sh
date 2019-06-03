#! /bin/bash

results_file=$1
tests_to_add=$(grep ' # TODO passed but expected fail' ${results_file} | sed -E 's/^ok [0-9]+ (\(expected fail\) )?//' | sed -E 's/( \([0-9]+ subtests\))? # TODO passed but expected fail$//')

fail_build=0
while read -r test_id; do
	grep "${test_id}" testfile > /dev/null 2>&1
	if [ "$?" != "0" ]; then
		echo "ERROR: Passed test not present in testfile: ${test_id}"
		fail_build=1
	else
		echo "WARN: Test in testfile still marked as expected fail: ${test_id}"
	fi
done <<< "${tests_to_add}"

exit ${fail_build}
