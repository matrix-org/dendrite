#!/usr/bin/env python3

from __future__ import division
import argparse
import re
import sys

# Usage: $ ./are-we-synapse-yet.py [-v] results.tap
# This script scans a results.tap file from Dendrite's CI process and spits out
# a rating of how close we are to Synapse parity, based purely on SyTests.
# The main complexity is grouping tests sensibly into features like 'Registration'
# and 'Federation'. Then it just checks the ones which are passing and calculates
# percentages for each group. Produces results like:
# 
# Client-Server APIs: 29% (196/666 tests)
# -------------------
#   Registration             :  62% (20/32 tests)
#   Login                    :   7% (1/15 tests)
#   V1 CS APIs               :  10% (3/30 tests)
#   ...
#
# or in verbose mode:
#
# Client-Server APIs: 29% (196/666 tests)
# -------------------
#  Registration             :  62% (20/32 tests)
#    ✓ GET /register yields a set of flows
#    ✓ POST /register can create a user
#    ✓ POST /register downcases capitals in usernames
#    ...
# 
# You can also tack `-v` on to see exactly which tests each category falls under.

test_mappings = {
    "nsp": "Non-Spec API",
    "unk": "Unknown API (no group specified)",
    "app": "Application Services API",
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
		"fsd": "Send-to-Device APIs",
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
        "dkb": "Device Key Backup",
        "xsk": "Cross-signing Keys",
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
		"jso": "Enforced canonical JSON",
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
#   verbose => True|False
# Produces:
# Client-Server APIs: 29% (196/666 tests)
# -------------------
#   Registration             :  62% (20/32 tests)
#   Login                    :   7% (1/15 tests)
#   V1 CS APIs               :  10% (3/30 tests)
#   ...
# or in verbose mode:
# Client-Server APIs: 29% (196/666 tests)
# -------------------
#  Registration             :  62% (20/32 tests)
#    ✓ GET /register yields a set of flows
#    ✓ POST /register can create a user
#    ✓ POST /register downcases capitals in usernames
#    ...
def print_stats(header_name, gid_to_tests, gid_to_name, verbose):
    subsections = [] # Registration: 100% (13/13 tests)
    subsection_test_names = {} # 'subsection name': ["✓ Test 1", "✓ Test 2", "× Test 3"]
    total_passing = 0
    total_tests = 0
    for gid, tests in gid_to_tests.items():
        group_total = len(tests)
        if group_total == 0:
            continue
        group_passing = 0
        test_names_and_marks = []
        for name, passing in tests.items():
            if passing:
                group_passing += 1
            test_names_and_marks.append(f"{'✓' if passing else '×'} {name}")
            
        total_tests += group_total
        total_passing += group_passing
        pct = "{0:.0f}%".format(group_passing/group_total * 100)
        line = "%s: %s (%d/%d tests)" % (gid_to_name[gid].ljust(25, ' '), pct.rjust(4, ' '), group_passing, group_total)
        subsections.append(line)
        subsection_test_names[line] = test_names_and_marks
    
    pct = "{0:.0f}%".format(total_passing/total_tests * 100)
    print("%s: %s (%d/%d tests)" % (header_name, pct, total_passing, total_tests))
    print("-" * (len(header_name)+1))
    for line in subsections:
        print("  %s" % (line,))
        if verbose:
            for test_name_and_pass_mark in subsection_test_names[line]:
                print("    %s" % (test_name_and_pass_mark,))
            print("")
    print("")

def main(results_tap_path, verbose):
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
        "appservice": {
            "app": {},
        },
        "nonspec": {
            "nsp": {},
            "unk": {}
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
                summary["nonspec"]["unk"][name] = test_result["ok"]
            if group_id == "nsp":
                summary["nonspec"]["nsp"][name] = test_result["ok"]
            elif group_id == "app":
                summary["appservice"]["app"][name] = test_result["ok"]
            elif group_id in test_mappings["federation_apis"]:
                group = summary["federation"].get(group_id, {})
                group[name] = test_result["ok"]
                summary["federation"][group_id] = group
            elif group_id in test_mappings["client_apis"]:
                group = summary["client"].get(group_id, {})
                group[name] = test_result["ok"]
                summary["client"][group_id] = group

    print("Are We Synapse Yet?")
    print("===================")
    print("")
    print_stats("Non-Spec APIs", summary["nonspec"], test_mappings, verbose)
    print_stats("Client-Server APIs", summary["client"], test_mappings["client_apis"], verbose)
    print_stats("Federation APIs", summary["federation"], test_mappings["federation_apis"], verbose)
    print_stats("Application Services APIs", summary["appservice"], test_mappings, verbose)



if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument("tap_file", help="path to results.tap")
    parser.add_argument("-v", action="store_true", help="show individual test names in output")
    args = parser.parse_args()
    main(args.tap_file, args.v)