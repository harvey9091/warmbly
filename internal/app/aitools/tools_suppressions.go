package aitools

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/warmbly/warmbly/internal/models"
	"github.com/warmbly/warmbly/internal/pkg/generation"
)

// Suppression-list tools. Reading is a contacts read; adding or lifting an
// entry decides who gets mail, so it takes the contacts write bit.
func (d Deps) registerSuppressionTools(r *Registry) {
	if d.Suppressions == nil {
		return
	}

	r.Register(Tool{
		Name:        "list_suppressions",
		Description: "List the workspace suppression list: the addresses and domains campaign mail is never sent to, and why each one is there. Use it to explain why a contact is not receiving mail.",
		InputSchema: objectSchema(map[string]any{
			"q":     strProp("Only entries whose address or domain contains this text."),
			"limit": intProp("Max entries (1-200, default 50)."),
		}),
		Risk:            generation.RiskRead,
		RequiredOrgPerm: models.PermViewContacts,
		RequiredAPIPerm: models.APIPermReadContacts,
		Handler:         d.listSuppressions,
	})

	r.Register(Tool{
		Name:        "add_suppressions",
		Description: "Add addresses or whole domains to the suppression list, so campaigns stop mailing them. A domain entry matches every address at it. Requires user approval.",
		InputSchema: objectSchema(map[string]any{
			"values": arrProp("Addresses, or bare domains with or without a leading @.", map[string]any{"type": "string"}),
			"reason": strProp("Why they are being suppressed, recorded on every entry."),
		}, "values"),
		Risk:            generation.RiskWrite,
		RequiredOrgPerm: models.PermManageContacts,
		RequiredAPIPerm: models.APIPermWriteContacts,
		Handler:         d.addSuppressions,
	})

	r.Register(Tool{
		Name:        "remove_suppression",
		Description: "Lift one suppression, letting campaigns mail that recipient again. Check why it was suppressed first: undoing somebody's own unsubscribe or a spam complaint is not something to do on a guess. Requires user approval.",
		InputSchema: objectSchema(map[string]any{
			"suppression_id": strProp("The suppression's UUID, from list_suppressions."),
		}, "suppression_id"),
		Risk:            generation.RiskWrite,
		RequiredOrgPerm: models.PermManageContacts,
		RequiredAPIPerm: models.APIPermWriteContacts,
		Handler:         d.removeSuppression,
	})
}

func (d Deps) listSuppressions(ctx context.Context, inv Invocation, args json.RawMessage) (string, error) {
	in, err := decodeArgs[struct {
		Q     string `json:"q"`
		Limit int    `json:"limit"`
	}](args)
	if err != nil {
		return "", err
	}
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, xerr := d.Suppressions.ListSuppressions(ctx, inv.OrgID, in.Q, nil, nil, limit)
	if xerr != nil {
		return "", fromErrx(xerr)
	}
	return jsonResult(map[string]any{"data": rows})
}

func (d Deps) addSuppressions(ctx context.Context, inv Invocation, args json.RawMessage) (string, error) {
	in, err := decodeArgs[struct {
		Values []string `json:"values"`
		Reason string   `json:"reason"`
	}](args)
	if err != nil {
		return "", err
	}
	if len(in.Values) == 0 {
		return "", ErrInvalidArgs
	}
	req := &models.AddSuppressionsRequest{Reason: in.Reason}
	for _, v := range in.Values {
		req.Entries = append(req.Entries, models.SuppressionEntry{Value: v})
	}
	res, xerr := d.Suppressions.AddSuppressions(ctx, inv.OrgID, inv.UserID, req)
	if xerr != nil {
		return "", fromErrx(xerr)
	}
	d.logAudit(ctx, inv, models.AuditActionCreate, models.AuditEntitySuppression, nil, map[string]string{
		"added": strconv.Itoa(res.Added), "skipped": strconv.Itoa(len(res.Skipped)),
	})
	return jsonResult(res)
}

func (d Deps) removeSuppression(ctx context.Context, inv Invocation, args json.RawMessage) (string, error) {
	in, err := decodeArgs[struct {
		SuppressionID string `json:"suppression_id"`
	}](args)
	if err != nil {
		return "", err
	}
	id, err := parseUUIDArg(in.SuppressionID)
	if err != nil {
		return "", err
	}
	entry, xerr := d.Suppressions.RemoveSuppression(ctx, inv.OrgID, id)
	if xerr != nil {
		return "", fromErrx(xerr)
	}
	// The row is gone, so what it held travels in the audit entry.
	d.logAudit(ctx, inv, models.AuditActionDelete, models.AuditEntitySuppression, &id, map[string]string{
		"value": entry.Email, "kind": string(entry.Kind), "source": string(entry.Source),
	})
	return jsonResult(map[string]any{"ok": true, "removed": entry})
}
