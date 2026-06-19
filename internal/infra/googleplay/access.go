package googleplay

import "context"

// CheckAccess verifies that the service account can act on the app by opening
// and immediately discarding an edit. It returns a classified error
// (ReasonPermission when the SA is not granted access or rights have not
// propagated yet, ReasonAuth for invalid credentials).
func (c *Client) CheckAccess(ctx context.Context, packageName string) error {
	editID, err := c.insertEdit(ctx, packageName)
	if err != nil {
		return classifyError(err)
	}
	c.abortEdit(ctx, packageName, editID)
	return nil
}
