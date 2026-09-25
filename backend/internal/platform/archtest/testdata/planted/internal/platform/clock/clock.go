package clock

import "time"

func Now() time.Time { return time.Now() }

func (c C) init() {}

type C struct{}
