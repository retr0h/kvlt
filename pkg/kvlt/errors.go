// Copyright (c) 2026 John Dewey

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package kvlt

import "errors"

// Sentinel errors callers can check with errors.Is. Wrapping the
// concrete failure (filesystem, AWS API, …) in one of these lets a
// caller distinguish "vault is missing" from "the key inside it is
// missing" without parsing strings.
var (
	// ErrVaultNotFound is returned when a vault by the given name is
	// not configured in the repository.
	ErrVaultNotFound = errors.New("vault not found")

	// ErrKeyNotFound is returned when a vault is configured but does
	// not contain the requested secret key.
	ErrKeyNotFound = errors.New("secret key not found")

	// ErrVaultAlreadyExists is returned when a vault with the given
	// name is already configured. Names are unique per repository.
	ErrVaultAlreadyExists = errors.New("vault already exists")

	// ErrInvalidConfig is returned when a vault config file fails
	// validation — malformed YAML, missing required setting, or
	// unknown backend type.
	ErrInvalidConfig = errors.New("invalid vault configuration")

	// ErrAuthFailed is returned when a backend rejects credentials.
	// The original error is wrapped — auth failures intentionally do
	// not expose the offending key in the message.
	ErrAuthFailed = errors.New("vault authentication failed")
)
