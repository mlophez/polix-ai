// File imports: a use case and another adapters package, which no adapters package imports (GOARCH-9).
package adapters

import (
	_ "example.com/app/internal/app/stock/features/count"
	_ "example.com/app/internal/app/stock/adapters"
)
