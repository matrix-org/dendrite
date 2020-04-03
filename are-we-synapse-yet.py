#!/usr/bin/env python3

# Usage: $ ./are-we-synapse-yet.py results.tap
# This script scans a results.tap file from Dendrite's CI process and spits out
# a rating of how close we are to Synapse parity, based purely on SyTests.
# The main complexity is grouping tests sensibly into features like 'Registration'
# and 'Federation'. Then it just checks the ones which are passing and calculates
# percentages for each group. Produces results like:
#
# Client-Server APIs: 43% (240/543 tests)
#    Registration: 100% (13/13 tests)
#    Login:         11% (9/85 tests)
#    ...
#
# Federation APIs: 22% (29/142 tests)
#    Key API:       55% (17/33 tests)
#    send_join API: 95% (10/11 tests)
#    ....
#
# Non-Spec APIs: (33%)
#
# You can also tack `-v` on to see exactly which tests each category falls under.

from __future__ import division
import re
import sys

test_mappings = {
    "nsp": "Non-Spec API",
    "f": "Federation", # flag to mark test involves federation

    "federation_apis": {
        "fky": "Key API",
        "fsj": "send_join API",
        "fmj": "make_join API",
        "fsl": "send_leave API",
        "fiv": "Invite API",
        "fqu": "Query API",
        "frv": "room versions",
        "fau": "Auth",
        "fbk": "Backfill API",
        "fme": "get_missing_events API",
        "fst": "State APIs",
        "fpb": "Public Room API",
        "fdk": "Device Key APIs",
        "fed": "Federation API",
    },

    "client_apis": {
        "reg": "Registration",
        "log": "Login",
        "lox": "Logout",
        "v1s": "V1 CS APIs",
        "csa": "Misc CS APIs",
        "pro": "Profile",
        "dev": "Devices",
        "dvk": "Device Keys",
        "pre": "Presence",
        "crm": "Create Room",
        "syn": "Sync API",
        "rmv": "Room Versions",
        "rst": "Room State APIs",
        "pub": "Public Room APIs",
        "mem": "Room Membership",
        "ali": "Room Aliases",
        "jon": "Joining Rooms",
        "lev": "Leaving Rooms",
        "inv": "Inviting users to Rooms",
        "ban": "Banning users",
        "snd": "Sending events",
        "get": "Getting events for Rooms",
        "rct": "Receipts",
        "red": "Read markers",
        "med": "Media APIs",
        "cap": "Capabilities API",
        "typ": "Typing API",
        "psh": "Push APIs",
        "acc": "Account APIs",
        "eph": "Ephemeral Events",
        "plv": "Power Levels",
        "xxx": "Redaction",
        "3pd": "Third-Party ID APIs",
        "gst": "Guest APIs",
        "ath": "Room Auth",
        "fgt": "Forget APIs",
        "ctx": "Context APIs",
        "upg": "Room Upgrade APIs",
        "tag": "Tagging APIs",
        "sch": "Search APIs",
        "oid": "OpenID API",
        "std": "Send-to-Device APIs",
        "adm": "Server Admin API",
        "ign": "Ignore Users",
        "udr": "User Directory APIs",
        "app": "Application Services API",
    },
}

# optional 'not ' with test number then anything but '#'
re_testname = re.compile(r"^(not )?ok [0-9]+ ([^#]+)")

# Parses lines like the following:
#
# SUCCESS:     ok 3 POST /register downcases capitals in usernames
# FAIL:        not ok 54 (expected fail) POST /createRoom creates a room with the given version
# SKIP:        ok 821 Multiple calls to /sync should not cause 500 errors # skip lack of can_post_room_receipts
# EXPECT FAIL: not ok 822 (expected fail) Guest user can call /events on another world_readable room (SYN-606) # TODO expected fail
#
# Only SUCCESS lines are treated as success, the rest are not implemented.
#
# Returns a dict like:
# { name: "...", ok: True }
def parse_test_line(line):
    if not line.startswith("ok ") and not line.startswith("not ok "):
        return
    re_match = re_testname.match(line)
    test_name = re_match.groups()[1].replace("(expected fail) ", "").strip()
    test_pass = False
    if line.startswith("ok ") and not "# skip " in line:
        test_pass = True
    return {
        "name": test_name,
        "ok": test_pass,
    }

# Prints the stats for a complete section.
#   header_name => "Client-Server APIs"
#   gid_to_tests => { gid: { <name>: True|False }}
#   gid_to_name  => { gid: "Group Name" }
# Produces:
# Client-Server APIs: 43% (240/543 tests)
#    Registration: 100% (13/13 tests)
#    Login:         11% (9/85 tests)
#    ...
def print_stats(header_name, gid_to_tests, gid_to_name):
    subsections = [] # Registration: 100% (13/13 tests)
    total_passing = 0
    total_tests = 0
    for gid, tests in gid_to_tests.items():
        group_total = len(tests)
        group_passing = 0
        for name, passing in tests.items():
            if passing:
                group_passing += 1
        total_tests += group_total
        total_passing += group_passing
        pct = "{0:.0f}%".format(group_passing/group_total * 100)
        line = "%s: %s (%d/%d tests)" % (gid_to_name[gid].rjust(30, ' '), pct.rjust(4, ' '), group_passing, group_total)
        subsections.append(line)
    
    pct = "{0:.0f}%".format(total_passing/total_tests * 100)
    print("%s: %s (%d/%d tests)" % (header_name, pct, total_passing, total_tests))
    for line in subsections:
        print("  %s" % (line,))

def main(results_tap_path):
    # Load up test mappings
    test_name_to_group_id = {}
    fed_tests = set()
    client_tests = set()
    with open("./are-we-synapse-yet.list", "r") as f:
        for line in f.readlines():
            test_name = " ".join(line.split(" ")[1:]).strip()
            groups = line.split(" ")[0].split(",")
            for gid in groups:
                if gid == "f" or gid in test_mappings["federation_apis"]:
                    fed_tests.add(test_name)
                else:
                    client_tests.add(test_name)
                if gid == "f":
                    continue # we expect another group ID
                test_name_to_group_id[test_name] = gid

    # parse results.tap
    summary = {
        "client": {
            # gid: {
            #   test_name: OK
            # }
        },
        "federation": {
            # gid: {
            #   test_name: OK
            # }
        },
        "nonspec": {
            "nsp": {}
        },
    }
    with open(results_tap_path, "r") as f:
        for line in f.readlines():
            test_result = parse_test_line(line)
            if not test_result:
                continue
            name = test_result["name"]
            group_id = test_name_to_group_id.get(name)
            if not group_id:
                raise Exception("The test '%s' doesn't have a group" % (name,))
            if group_id == "nsp":
                summary["nonspec"]["nsp"][name] = test_result["ok"]
            elif group_id in test_mappings["federation_apis"]:
                group = summary["federation"].get(group_id, {})
                group[name] = test_result["ok"]
                summary["federation"][group_id] = group
            elif group_id in test_mappings["client_apis"]:
                group = summary["client"].get(group_id, {})
                group[name] = test_result["ok"]
                summary["client"][group_id] = group

    print_stats("Non-Spec APIs", summary["nonspec"], test_mappings)
    print_stats("Client-Server APIs", summary["client"], test_mappings["client_apis"])
    print_stats("Federation APIs", summary["federation"], test_mappings["federation_apis"])



if __name__ == '__main__':
    main(sys.argv[1])