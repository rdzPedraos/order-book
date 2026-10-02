// Package orderdb reads and writes the orders table with hand-written SQL
// through Gofr's ctx.SQL. Handlers call its functions directly; tests replace
// the database with InitMock.
package orderdb
