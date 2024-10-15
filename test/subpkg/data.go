package subpkg

var DataStr string

func init() {
	DataStr = "Hello World"
}

func Data() string {
	return DataStr
}
