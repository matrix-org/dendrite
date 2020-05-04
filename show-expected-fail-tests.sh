#! /bin/bash
#
# Parses a results.tap file from SyTest output and a file containing test names (a test whitelist)
# and checks whether a test name that exists in the whitelist (that should pass), failed or not.
#
# An optional blacklist file can be added, also containing test names, where if a test name is
# present, the script will not error even if the test is in the whitelist file and failed
#
# For each of these files, lines starting with '#' are ignored.
#
# Usage ./show-expected-fail-tests.sh results.tap whitelist [blacklist]

results_file=$1
whitelist_file=$2
blacklist_file=$3

fail_build=0

if [ $# -lt 2 ]; then
	echo "Usage: $0 results.tap whitelist [blacklist]"
	exit 1
fi

if [ ! -f "$results_file" ]; then
	echo "ERROR: Specified results file '${results_file}' doesn't exist."
	fail_build=1
fi

if [ ! -f "$whitelist_file" ]; then
	echo "ERROR: Specified test whitelist '${whitelist_file}' doesn't exist."
	fail_build=1
fi

blacklisted_tests=()

# Check if a blacklist file was provided
if [ $# -eq 3 ]; then
	# Read test blacklist file
	if [ ! -f "$blacklist_file" ]; then
		echo "ERROR: Specified test blacklist file '${blacklist_file}' doesn't exist."
		fail_build=1
	fi

	# Read each line, ignoring those that start with '#'
	blacklisted_tests=""
	search_non_comments=$(grep -v '^#' ${blacklist_file})
	while read -r line ; do
		# Record the blacklisted test name
		blacklisted_tests+=("${line}")
	done <<< "${search_non_comments}"  # This allows us to edit blacklisted_tests in the while loop
fi

[ "$fail_build" = 0 ] || exit 1

passed_but_expected_fail=$(grep ' # TODO passed but expected fail' ${results_file} | sed -E 's/^ok [0-9]+ (\(expected fail\) )?//' | sed -E 's/( \([0-9]+ subtests\))? # TODO passed but expected fail$//')
tests_to_add=""
already_in_whitelist=""

while read -r test_name; do
	# Ignore empty lines
	[ "${test_name}" = "" ] && continue

	grep "^${test_name}" "${whitelist_file}" > /dev/null 2>&1
	if [ "$?" != "0" ]; then
		# Check if this test name is blacklisted
		if printf '%s\n' "${blacklisted_tests[@]}" | grep -q -P "^${test_name}$"; then
			# Don't notify about this test
			continue
		fi

		# Append this test_name to the existing list
		tests_to_add="${tests_to_add}${test_name}\n"
		fail_build=1
	else
		already_in_whitelist="${already_in_whitelist}${test_name}\n"
	fi
done <<< "${passed_but_expected_fail}"

# TODO: Check that the same test doesn't exist in both the whitelist and blacklist
# TODO: Check that the same test doesn't appear twice in the whitelist|blacklist

# Trim test output strings
tests_to_add=$(IFS=$'\n' echo "${tests_to_add[*]%%'\n'}")
already_in_whitelist=$(IFS=$'\n' echo "${already_in_whitelist[*]%%'\n'}")

# Format output with markdown for buildkite annotation rendering purposes
if [ -n "${tests_to_add}" ] && [ -n "${already_in_whitelist}" ]; then
	echo "### 📜 SyTest Whitelist Maintenance"
fi

if [ -n "${tests_to_add}" ]; then
	echo "**ERROR**: The following tests passed but are not present in \`$2\`. Please append them to the file:"
    echo "\`\`\`"
    echo -e "${tests_to_add}"
    echo "\`\`\`"
fi

if [ -n "${already_in_whitelist}" ]; then
	echo "**WARN**: Tests in the whitelist still marked as **expected fail**:"
    echo "\`\`\`"
    echo -e "${already_in_whitelist}"
    echo "\`\`\`"
fi

exit ${fail_build}
