-- A campaign fed by a live lead source waits for leads instead of finishing
-- (issue #340). Segments were covered by 000130; forms and automations that
-- enrol leads are the same case.
UPDATE public.campaigns SET continuous = true
WHERE NOT continuous
  AND id IN (SELECT campaign_id FROM public.forms WHERE campaign_id IS NOT NULL);

UPDATE public.campaigns SET continuous = true
WHERE NOT continuous
  AND id IN (
    SELECT (n->'config'->>'campaign_id')::uuid
    FROM public.automations a,
         jsonb_array_elements(a.graph->'nodes') n
    WHERE n->>'type' = 'action'
      AND n->>'action' IN ('warmbly.add_to_campaign', 'warmbly.upsert_contact')
      AND n->'config'->>'campaign_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
  );
