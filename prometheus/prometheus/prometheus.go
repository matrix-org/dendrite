package prometheus

type CounterOpts struct {
	Name string
	Help string
}
type SummaryOpts struct {
	Namespace string
	Subsystem string
	Name      string
	Help      string
}

type CounterVec struct{}

type Collector interface {
	WithLabelValues(a string, b string) Collector
	Observe(c float64)
	Inc()
}

type col struct{}

func (c col) WithLabelValues(a, b string) Collector {
	return c
}
func (c col) Observe(d float64) {}
func (c col) Inc()              {}

func NewSummaryVec(opts SummaryOpts, arr []string) Collector {
	return col{}
}

func MustRegister(...Collector) {}

func NewCounter(opt CounterOpts) col {
	return col{}
}
