# Tonnerre

A simple HTTP load generator, nothing fancy.

# Customize

The code is straight forward enough to let you collect more metrics in
`func consumeResponses(resp <-chan Resp) <-chan struct{}`, and by adding
fields to `type Resp`.
