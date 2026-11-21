// A test of the domain imports an adapters package, which GOARCH-9 admits to every test of internal/app/.
package stock

import _ "example.com/app/internal/app/stock/adapters"
