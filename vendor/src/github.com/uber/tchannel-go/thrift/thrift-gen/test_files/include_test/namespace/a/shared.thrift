namespace go a_shared

include "../b/shared.thrift"

typedef shared.b_string a_string

service AShared extends shared.BShared {
	bool healthA()
}