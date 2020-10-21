#!/bin/bash
#############################################
#Script to generate Systemd-Unit Files      #
#Author: dymattic(@dymattic:schwifty.earth) #
#############################################
check_parameters() {
    if [[ -z "${1:-}" ]]; then
      echo "The specified command requires additional parameters. See help:" >&2
      echo >&2
      command_help >&2
      exit 1
    elif [[ "${1:0:1}" = "-" ]]; then
      _exiterr "Invalid argument: ${1}"
    fi
  }
print_param_options()
{

	echo "Available servers:"
	for entry in "${SERVERS[@]}"; do
		echo "    - $entry"
	done
}


#Array with all servers
SERVERS=( "clientapi" \
	"syncapi" \
	"mediaapi" \
	"federationapi" \
	"roomserver" \
	"federationsender" \
	"appservice" \
	"keyserver" \
	"signingkeyserver" \
	"eduserver" \
	"userapi" \
	)

#Generate unit files and output into /etc/systemd/system/<unit-file-name>
function generateServiceUnit()
{



	#Check if already existent

	if [[ -f /etc/systemd/system/dendrite@.service && ( "$PARAM_FORCE" != "yes" || -z "$PARAM_FORCE" ) ]]; then
		echo "Systemd-Unit-Template file \"dendrite@.service\" already exists."
		echo "Add \"-x|--force\" to overwrite!"
		return 0
	fi
	cat <<-EOF > /etc/systemd/system/dendrite@.service
	[Unit]
	Description= Dendrite PolyLith Multi - %I
	PartOf=polyDendrite.service
	After=network.target

	[Service]
	User=$USER
	WorkingDirectory=$DENDRITEDIR
	Type=forking
	ExecStart=$DENDRITEDIR/bin/dendrite-polylith-multi --config=dendrite.yaml %i
	Restart=on-failure

	[Install]
	WantedBy=multi-user.target
EOF

	if [[ $? -ne 0 ]]; then
		echo "An error occurred. Exiting..."
		exit 1
	fi
	/usr/bin/systemctl daemon-reload
	echo "Enabling dendrite@.service..."
	/usr/bin/systemctl enable dendrite@.service
	return 0
}

function command_help()
{
	echo "\
        --help|-h  - Display this message
        --user|-u username - Specify username that executes the servers
        --dir|-d /path/to/dendrite/dir - Set the path of the dendrite directory i.e. (/home/dendrite/)
        [server-name][...] or all for all - Give list of server-names to install Systemd units for or leave empty for all"
return 0
}
declare -a inputarray
while (( ${#} )); do
    case "${1}" in
      --help|-h)
        command_help
        exit 0
        ;;

      # PARAM_Usage: --user (-u) username
      # PARAM_Description: Use specified user for service execution
      --user|-u)
        shift 1
        check_parameters "${1:-}"
        USER="${1}"
        ;;

      # PARAM_Usage: --dir (-d) /path/to/dendrite/dir
      # PARAM_Description: Use specified path to dendrite directory.
      --dir|-d)
        shift 1
        check_parameters "${1:-}"
        DENDRITEDIR="${1}"
        ;;

      # PARAM_Usage: --force (-x)
      # PARAM_Description: Force overwrite, don't ask
      --force|-x)
        PARAM_FORCE="yes"
        ;;

      -*)
        echo "Unknown parameter detected: ${1}"
        command_help
        exit 1
        ;;

      *)
        available=( ${SERVERS[@]} "all" )
		if [[ ! " ${available[@]} " =~ " ${1} " ]]; then
			echo "Invalid argument: ${1}"
			print_param_options
			exit 1
		fi
		inputarray+=( "${1}" )
		;;
    esac

    shift 1
  done


#Get user
if [[ -z "$USER" ]]; then
	read -p "Enter username of user running dendrite: " USER
	if [[ $USER = "" ]]; then
		echo "Dendrite-User can not be empty..."
		exit 1
	fi
fi
#Get install dir
if [[ -z "$DENDRITEDIR" ]]; then
	read -p "Enter path to dendrite-directory: " DENDRITEDIR
	if [[ DENDRITEDIR = "" ]]; then
		echo "Dendrite-Path can not be empty..."
		exit 1
	elif [[ ! -d $DENDRITEDIR/bin ]]; then
		echo "There is no \"bin\"-Dir in $DENDRITEDIR. Right path? Exiting..."
	fi
fi
DENDRITEDIR="$(echo $DENDRITEDIR | sed -e 's#/$##')"
#Let user decide which servers to create the unitfiles for...
if [[ -z "${inputarray[@]}" || "${inputarray[@]}" = "" ]]; then
	echo "Enter a list of dendrite-servers separated by spaces to create Systemd-Units for or enter \"all\" to create all:"
	print_param_options
	while read -a inputarray; do
	  echo ${inputarray[1]}
	done
fi

generateServiceUnit

if [[ "${inputarray[@]}" = "" || "${inputarray[@]}" = "all" ]]; then
    for part in "${SERVERS[@]}"; do
    	/usr/bin/systemctl enable dendrite@${part}.service &&	/usr/bin/systemctl start dendrite@${part}.service
    done
else
    for choice in "${inputarray[@]}"; do
    	/usr/bin/systemctl enable dendrite@${choice}.service &&	/usr/bin/systemctl start dendrite@${choice}.service
    done
fi

echo "Done.."
echo "You can start all Services by typing:
systemctl start dendrite@*"
echo "To start single servers use:
systemctl start dendrite@<servicename>
"
exit 0