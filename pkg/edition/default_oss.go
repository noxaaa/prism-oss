package edition

func init() {
	MustRegisterProvider(OSSProvider())
}

func defaultKey() Key {
	return KeyOSS
}
