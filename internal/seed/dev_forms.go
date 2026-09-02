package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Dev-org forms ("dddd0f" block).
var (
	devFormDemo       = uuid.MustParse("33333333-0000-0000-0000-000000000001")
	devFormNewsletter = uuid.MustParse("33333333-0000-0000-0000-000000000002")
	devFormContact    = uuid.MustParse("33333333-0000-0000-0000-000000000003")
)

// devFormDemoPublicID is referenced from the draft campaign's step, so the
// campaign Forms panel has something to report before anything is sent.
const devFormDemoPublicID = "bkademe23456789abcdef"

func devFormLinkID(i int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("dddd0f00-0000-0000-0000-%012d", i))
}

func devFormEventID(journey, step int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("dddd0f01-0000-0000-0000-%06d%06d", journey, step))
}

func devFormSubmissionID(i int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("dddd0f02-0000-0000-0000-%012d", i))
}

// seedDevForms builds the hosted-forms fixture: a three-page demo form on the
// split layout, a one-question newsletter form in focus mode and a draft,
// plus the personalized links, funnel events and submissions that make the
// analytics, responses and campaign panels show real shapes.
func seedDevForms(ctx context.Context, pool *pgxpool.Pool) error {
	demoFields := `[
		{"id":"f_head","type":"heading","label":"Book a demo","required":false},
		{"id":"f_intro","type":"paragraph","label":"","value":"Twenty minutes, no slides. We will look at your current sending setup together.","required":false},
		{"id":"f_first","type":"text","label":"First name","required":true,"map_to":"first_name","width":"half"},
		{"id":"f_last","type":"text","label":"Last name","required":false,"map_to":"last_name","width":"half"},
		{"id":"f_email","type":"email","label":"Work email","required":true,"map_to":"email"},
		{"id":"f_break1","type":"page_break","label":"About your team","required":false},
		{"id":"f_company","type":"text","label":"Company","required":false,"map_to":"company"},
		{"id":"f_size","type":"select","label":"How many people send outbound?","required":true,"options":["Just me","2-10","11-50","50+"]},
		{"id":"f_mailboxes","type":"number","label":"Mailboxes in rotation","required":false},
		{"id":"f_break2","type":"page_break","label":"Anything else","required":false},
		{"id":"f_note","type":"textarea","label":"What would you like to cover?","required":false,"rows":4},
		{"id":"f_terms","type":"checkbox","label":"Terms","placeholder":"I agree to be contacted about this request","required":true}
	]`
	demoDesign := `{"theme":"ocean","layout":"split","mode":"classic","align":"left","show_progress":true,
		"font_family":"sora","page_background":"#0c4a6e","page_background_end":"#082f49","form_background":"#ffffff",
		"text_color":"#0f172a","label_color":"#334155","input_background":"#ffffff","input_border_color":"#e2e8f0",
		"input_text_color":"#0f172a","placeholder_color":"#94a3b8","accent_color":"#0284c7",
		"button_background":"#0284c7","button_text_color":"#ffffff","button_text":"Book my demo",
		"border_radius":12,"shadow":true}`

	newsletterFields := `[
		{"id":"n_email","type":"email","label":"Email","required":true,"map_to":"email"},
		{"id":"n_topic","type":"radio","label":"What should we send you?","required":true,"options":["Product news","Deliverability tips","Both"]}
	]`
	newsletterDesign := `{"theme":"midnight","layout":"card","mode":"focus","align":"center","show_progress":true,
		"font_family":"inter","page_background":"#0f172a","form_background":"#1e293b","text_color":"#f1f5f9",
		"label_color":"#cbd5e1","input_background":"#0f172a","input_border_color":"#334155","input_text_color":"#f1f5f9",
		"placeholder_color":"#64748b","accent_color":"#38bdf8","button_background":"#38bdf8","button_text_color":"#0f172a",
		"button_text":"Subscribe","border_radius":12,"shadow":false}`

	contactFields := `[
		{"id":"c_name","type":"text","label":"Name","required":true,"map_to":"first_name"},
		{"id":"c_email","type":"email","label":"Email","required":true,"map_to":"email"},
		{"id":"c_msg","type":"textarea","label":"How can we help?","required":true,"rows":5}
	]`

	forms := []struct {
		id       uuid.UUID
		publicID string
		name     string
		status   string
		fields   string
		design   string
		views    int64
		ageDays  int
	}{
		{devFormDemo, devFormDemoPublicID, "Book a demo", "published", demoFields, demoDesign, 412, 35},
		{devFormNewsletter, "newsjxyzsignup2345678", "Newsletter signup", "published", newsletterFields, newsletterDesign, 168, 52},
		{devFormContact, "cntactus23456789abcde", "Contact us", "draft", contactFields, `{"theme":"minimal","layout":"card","mode":"classic"}`, 0, 3},
	}

	for _, f := range forms {
		var publishedAt any
		if f.status == "published" {
			publishedAt = fmt.Sprintf("%d days", f.ageDays-1)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO forms (
				id, organization_id, created_by, public_id, name, status, fields, design,
				success_message, captcha_enabled, views_count, published_at, created_at, updated_at
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7::jsonb,$8::jsonb,$9,false,$10,
				CASE WHEN $11::text IS NULL THEN NULL ELSE now() - $11::interval END,
				now() - make_interval(days => $12), now()
			)
			ON CONFLICT (id) DO UPDATE SET
				public_id = EXCLUDED.public_id,
				name = EXCLUDED.name,
				status = EXCLUDED.status,
				fields = EXCLUDED.fields,
				design = EXCLUDED.design,
				views_count = EXCLUDED.views_count,
				updated_at = now()
		`, f.id, DevOrgID, DevUserID, f.publicID, f.name, f.status, f.fields, f.design,
			"Thanks! We will be in touch shortly.", f.views, publishedAt, f.ageDays); err != nil {
			return fmt.Errorf("form %s: %w", f.name, err)
		}
	}

	// Demo-form leads are filed under Leads; newsletter signups under Customers.
	for _, b := range []struct {
		form, category uuid.UUID
	}{
		{devFormDemo, devCategoryLead},
		{devFormNewsletter, devCategoryCustomer},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO form_categories (form_id, category_id) VALUES ($1,$2)
			ON CONFLICT DO NOTHING
		`, b.form, b.category); err != nil {
			return fmt.Errorf("form category: %w", err)
		}
	}

	return seedDevFormFunnel(ctx, pool)
}

// seedDevFormFunnel walks seven contacts of the active campaign through the
// demo form: all of them opened their personalized link, most started, fewer
// reached the later pages and three finished. The ones who stalled are what
// the responses table shows as "in progress".
func seedDevFormFunnel(ctx context.Context, pool *pgxpool.Pool) error {
	journeys := []struct {
		contact      int
		furthestPage int
		submitted    bool
		country      string
		device       string
	}{
		{0, 2, true, "US", "desktop"},
		{1, 2, true, "GB", "mobile"},
		{2, 2, true, "US", "desktop"},
		{3, 1, false, "DE", "desktop"},
		{4, 1, false, "US", "mobile"},
		{5, 0, false, "CA", "tablet"},
		{6, 0, false, "US", "desktop"},
	}

	for i, j := range journeys {
		contact := devContactID(j.contact)
		if _, err := pool.Exec(ctx, `
			INSERT INTO form_links (id, form_id, organization_id, contact_id, campaign_id, created_at)
			VALUES ($1,$2,$3,$4,$5, now() - make_interval(days => $6))
			ON CONFLICT (form_id, contact_id) DO NOTHING
		`, devFormLinkID(i+1), devFormDemo, DevOrgID, contact, DevCampaignActiveID, 20-i); err != nil {
			return fmt.Errorf("form link %d: %w", i, err)
		}

		// A journey is a view, then a start, then one page event per page
		// reached, exactly as the hosted app reports them.
		type ev struct {
			kind string
			page int
		}
		events := []ev{{"view", 0}}
		if j.furthestPage > 0 || j.submitted {
			events = append(events, ev{"start", 0})
		}
		for p := 1; p <= j.furthestPage; p++ {
			events = append(events, ev{"page", p})
		}
		if j.submitted {
			events = append(events, ev{"submit", j.furthestPage})
		}

		for k, e := range events {
			if _, err := pool.Exec(ctx, `
				INSERT INTO form_events (
					id, organization_id, form_id, event_type, occurred_at, visitor_key,
					page_index, pages_total, contact_id, campaign_id, referrer_domain, country_code, device
				) VALUES (
					$1,$2,$3,$4, now() - make_interval(days => $5, mins => $6), $7,$8,3,$9,$10,
					'mail.google.com',$11,$12
				)
				ON CONFLICT (id) DO NOTHING
			`, devFormEventID(i+1, k+1), DevOrgID, devFormDemo, e.kind, 19-i, 90-k*9,
				fmt.Sprintf("dev-vk-%d", i+1), e.page, contact, DevCampaignActiveID,
				j.country, j.device); err != nil {
				return fmt.Errorf("form event %d/%d: %w", i, k, err)
			}
		}

		if !j.submitted {
			continue
		}
		c := devContacts[j.contact]
		data := fmt.Sprintf(
			`{"f_first":%q,"f_last":%q,"f_email":%q,"f_company":%q,"f_size":"11-50","f_note":"Mostly worried about landing in spam as we scale.","f_terms":"true"}`,
			c.first, c.last, devContactEmail(c), c.company)
		if _, err := pool.Exec(ctx, `
			INSERT INTO form_submissions (
				id, form_id, organization_id, contact_id, campaign_id, data, source_url, created_at
			) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7, now() - make_interval(days => $8, hours => 2))
			ON CONFLICT (id) DO NOTHING
		`, devFormSubmissionID(i+1), devFormDemo, DevOrgID, contact, DevCampaignActiveID,
			data, "https://forms.warmbly.com/f/"+devFormDemoPublicID, 18-i); err != nil {
			return fmt.Errorf("form submission %d: %w", i, err)
		}
	}

	// Keep the denormalized counters in step with the rows written above.
	if _, err := pool.Exec(ctx, `
		UPDATE forms f SET
			submissions_count = (SELECT COUNT(*) FROM form_submissions s WHERE s.form_id = f.id),
			last_submission_at = (SELECT MAX(created_at) FROM form_submissions s WHERE s.form_id = f.id)
		WHERE f.organization_id = $1
	`, DevOrgID); err != nil {
		return fmt.Errorf("form counters: %w", err)
	}
	return nil
}
