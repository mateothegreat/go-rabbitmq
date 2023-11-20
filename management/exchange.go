package management

type Exchange struct {
	Name    string
	Type    string
	Durable bool
	Queues  []Queue
}
