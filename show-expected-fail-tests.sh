#! /bin/bash

results_file=$1
testfile=$2

fail_build=0

if [ ! -f "$results_file" ]; then
	echo "ERROR: Specified results file ${results_file} doesn't exist."
	fail_build=1
fi

if [ ! -f "$testfile" ]; then
	echo "ERROR: Specified testfile ${testfile} doesn't exist."
	fail_build=1
fi

[ "$fail_build" = 0 ] || exit 1

passed_but_expected_fail=$(grep ' # TODO passed but expected fail' ${results_file} | sed -E 's/^ok [0-9]+ (\(expected fail\) )?//' | sed -E 's/( \([0-9]+ subtests\))? # TODO passed but expected fail$//')
tests_to_add=""
already_in_testfile=""

while read -r test_id; do
	[ "${test_id}" = "" ] && continue
	grep "${test_id}" "${testfile}" > /dev/null 2>&1
	if [ "$?" != "0" ]; then
		tests_to_add="${tests_to_add}${test_id}\n"
		fail_build=1
	else
		already_in_testfile="${already_in_testfile}${test_id}\n"
	fi
done <<< "${passed_but_expected_fail}"

if [ -n "${tests_to_add}" ]; then
	echo "ERROR: The following passed tests are not present in testfile. Please append them to the file:"
	echo -e "${tests_to_add}"
fi

if [ -n "${already_in_testfile}" ]; then
	echo "WARN: Tests in testfile still marked as expected fail:"
	echo -e "${already_in_testfile}"
fi

exit ${fail_build}
