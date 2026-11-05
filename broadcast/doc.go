// Package broadcast lets a Go application work with a Pusher-protocol server
// such as reverb-go or Laravel Reverb. It can publish events and answer the
// /broadcasting/auth request for private and presence channels.
//
// Event names and payloads must match what Laravel would send. For a Laravel
// event class use its full class name (for example `App\Events\CallEnded`), or
// the value returned by broadcastAs(). Echo's listen('CallEnded') matches
// `App\Events\CallEnded`, and listen('.custom') matches the name `custom`.
// The payload is the event's public properties at the top level, or the array
// returned by broadcastWith().
package broadcast
