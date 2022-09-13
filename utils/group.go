package utils

type Message struct {
	Index int
	Msg   string
}

func NewGroupE[T chan struct{}](chans [4]T) (group chan int) {
	group = make(chan int)
	for i, ch := range chans {
		go func(c T, i int) {
			for range c {
				group <- i
			}
		}(ch, i)
	}
	return
}

func NewGroupV[T chan string](chans [4]T) (group chan Message) {
	group = make(chan Message)
	for i, ch := range chans {
		go func(c T, i int) {
			for range c {
				group <- Message{i, <-c}
			}
		}(ch, i)
	}
	return
}
