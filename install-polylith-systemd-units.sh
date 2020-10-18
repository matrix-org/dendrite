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
SERVERS=( "client-api-proxy" \
	"federation-api-proxy" \
	"dendrite-client-api-server" \
	"dendrite-sync-api-server" \
	"dendrite-media-api-server" \
	"dendrite-federation-api-server" \
	"dendrite-room-server" \
	"dendrite-federation-sender-server" \
	"dendrite-appservice-server" \
	"dendrite-key-server" \
	"dendrite-signing-key-server" \
	"dendrite-edu-server" \
	"dendrite-user-api-server" \
	)

#Generate unit files and output into /etc/systemd/system/<unit-file-name>
function generateServiceUnit()
{
	declare SERVER=$1
	declare USER=$2
	declare DENDRITEDIR=$3

	#
	# Declaration of executin lines
	#
	declare -A EXECS
	nl=$'\n'
	read -r -d '' EXECS[clientapiproxy] <<EOF
	/bin/client-api-proxy \
	--bind-address ":8008" \
	--client-api-server-url "http://localhost:7771" \
	--sync-api-server-url "http://localhost:7773" \
	--media-api-server-url "http://localhost:7774" \
EOF


	read -r -d '' EXECS[federationapiproxy] <<EOF
	/bin/federation-api-proxy \ $nl
	--bind-address ":8448" \ $nl
	--federation-api-url "http://localhost:7772" \ $nl
	--media-api-server-url "http://localhost:7774" \ $nl
EOF


	for ENTRY in $DENDRITEDIR/bin/dendrite-*-server; do
	    BINARY=$(basename $ENTRY)
	    EXECS[$(echo "${BINARY//-}")]="$ENTRY --config=dendrite.yaml"
	done


	#Check if already existent
	CHECK=0
	if [[ -f /etc/systemd/system/$SERVER.service && ( "$PARAM_FORCE" != "yes" || -z "$PARAM_FORCE" ) ]]; then
		echo "Systemd-Unit file for $SERVER already exists."
		while [[ "$CHECK" -ne 1 ]]
		do
			read -p " Overwrite? (Y/N):" -n1 -r INPUT
	  		if [[ $INPUT =~ ^[Nn]$ ]]; then
				echo "Skipping...";
				return 0
			elif [[ $INPUT =~ ^[Yy]$ ]]; then
				CHECK=1
			else
				echo "Invalid input, try again!"
				CHECK=0
			fi
		done
	fi
	current="${EXECS[$(echo "${SERVER//-}")]}"
	cat <<-EOF > /etc/systemd/system/$SERVER.service
	[Unit]
	Description= $SERVER 1
	# When systemd stops or restarts the app.service, the action is propagated to this unit
	PartOf=polyDendrite.service
	# Start this unit after the app.service start
	After=polyDendrite.service

	[Service]
	User=$USER
	WorkingDirectory=$DENDRITEDIR
	# Pretend that the component is running
	ExecStart=/home/dendrite/server$current
	Restart=on-failure

	[Install]
	# This unit should start when app.service is starting
	WantedBy=polyDendrite.service
EOF

	if [[ $? -ne 0 ]]; then
		echo "An error occurred. Exiting..."
		exit 1
	fi
	/usr/bin/systemctl daemon-reload
	echo "Enabling $SERVER.service..."
	/usr/bin/systemctl enable $SERVER.service
	echo "$SERVER Unit-File created..."
	return 0
}

function generateServiceWrapper()
{
	if [[ -f /etc/systemd/system/polyDendrite.service && ( "$PARAM_FORCE" != "yes" || -z "$PARAM_FORCE" ) ]]; then
	echo "Systemd-Unit file for Dendrite Polylith Service wrapper already exists."
	CHECK=0
	while [[ "$CHECK" -ne 1 ]]
	do
		read -p " Overwrite? (Y/N):" -n1 -r INPUT
  		if [[ $INPUT =~ ^[Nn]$ ]]; then
  			echo
			echo "Skipping...";
			return 0
		elif [[ $INPUT =~ ^[Yy]$ ]]; then
			CHECK=1
		else
			echo "Invalid input, try again!"
			CHECK=0
		fi
	done
	fi
	read -r -d '' polyUnit <<EOF
[Unit]
Description=Dendrite Polylith Service wrapper

[Service]
# The dummy program will exit
Type=oneshot
# Execute a dummy program
ExecStart=/bin/true
# This service shall be considered active after start
RemainAfterExit=yes

[Install]
# Components of this application should be started at boot time
WantedBy=multi-user.target
EOF
	echo "$polyUnit" > /etc/systemd/system/polyDendrite.service

	if [[ $? -ne 0 ]]; then
		echo "An error occurred. Exiting..."
		exit 1
	fi
	/usr/bin/systemctl daemon-reload
	echo "Enabling polyDendrite.service..."
	/usr/bin/systemctl enable polyDendrite.service
	echo "$SERVER Unit-File created..."
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


if [[ "${inputarray[@]}" = "" || "${inputarray[@]}" = "all" ]]; then
    for part in "${SERVERS[@]}"; do
    	echo "$( generateServiceUnit $part $USER $DENDRITEDIR )"
    done
else
    for choice in "${inputarray[@]}"; do
        echo "$( generateServiceUnit $choice $USER $DENDRITEDIR )"
    done
fi
generateServiceWrapper

echo "Done.."
echo "You can start all Services by typing:
systemctl start polyDendrite.service"
echo "To start single servers use the appropriate service name instead."
exit 0