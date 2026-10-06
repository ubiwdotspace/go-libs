package logging

import "io"

const startupBanner = `
██╗   ██╗██████╗ ██╗██╗    ██╗
██║   ██║██╔══██╗██║██║    ██║
██║   ██║██████╔╝██║██║ █╗ ██║
██║   ██║██╔══██╗██║██║███╗██║
╚██████╔╝██████╔╝██║╚███╔███╔╝
 ╚═════╝ ╚═════╝ ╚═╝ ╚══╝╚══╝

`

// PrintBanner writes the UBIW startup banner. Use stderr to keep stdout logs structured.
func PrintBanner(output io.Writer) error {
	_, err := io.WriteString(output, startupBanner)
	return err
}
