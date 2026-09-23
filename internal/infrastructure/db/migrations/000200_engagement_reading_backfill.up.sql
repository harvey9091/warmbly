-- Re-reads the opens and clicks logged before 000199 by the rules the
-- consumer now applies (internal/pkg/mailclient). An image proxy's user
-- agent describes the proxy, so those rows lose the device, browser and, for
-- a data-centre proxy, the location they were given; installed mail apps
-- and webmail gain their client_type. Each statement only touches rows it
-- changes, so a rerun is a no-op.

-- Data-centre proxies: nothing about the reader survives.
UPDATE email_opens
SET client = CASE
        WHEN user_agent ILIKE '%googleimageproxy%' OR user_agent ILIKE '%via ggpht.com%' THEN 'Gmail'
        WHEN user_agent ILIKE '%yahoomailproxy%' THEN 'Yahoo Mail'
        WHEN user_agent ILIKE '%hey.com/imageproxy%' THEN 'HEY'
        WHEN user_agent ILIKE '%fastmailua%' THEN 'Fastmail'
        ELSE 'Seznam Email' END,
    client_type = '', device_hidden = true,
    device_type = '', os = '', browser = '', browser_version = '',
    country_code = '', region = '', city = ''
WHERE NOT device_hidden
  AND (user_agent ILIKE '%googleimageproxy%' OR user_agent ILIKE '%via ggpht.com%'
       OR user_agent ILIKE '%yahoomailproxy%' OR user_agent ILIKE '%hey.com/imageproxy%'
       OR user_agent ILIKE '%fastmailua%' OR user_agent ILIKE '%seznamemailproxy%');

-- Apple Mail Privacy Protection: the relay keeps the region, not the city.
UPDATE email_opens
SET client = 'Apple Mail', client_type = '', device_hidden = true,
    device_type = '', os = '', browser = '', browser_version = '', city = ''
WHERE NOT device_hidden AND lower(btrim(user_agent)) = 'mozilla/5.0';

-- The stripped WebKit signature: Mac Mail or the new Outlook, on a device
-- the string cannot name. The parser used to take its platform for a browser.
UPDATE email_opens
SET client = 'Apple Mail or Outlook', client_type = 'app', device_hidden = false,
    device_type = '', os = '', browser = '', browser_version = ''
WHERE client IS DISTINCT FROM 'Apple Mail or Outlook'
  AND lower(btrim(user_agent)) LIKE '%applewebkit/%'
  AND lower(btrim(user_agent)) LIKE '%(khtml, like gecko)';

UPDATE email_link_clicks
SET client = '', client_type = '', device_type = '', os = '', browser = '', browser_version = ''
WHERE (client <> '' OR browser <> '' OR os <> '')
  AND lower(btrim(user_agent)) LIKE '%applewebkit/%'
  AND lower(btrim(user_agent)) LIKE '%(khtml, like gecko)';

-- Clients that name themselves are installed apps.
UPDATE email_opens
SET client = CASE
        WHEN user_agent ILIKE '%thunderbird/%' THEN 'Thunderbird'
        WHEN user_agent ILIKE '%em client%' THEN 'eM Client'
        WHEN user_agent ILIKE '%mailbird/%' THEN 'Mailbird'
        WHEN user_agent ILIKE '%mailspring/%' THEN 'Mailspring'
        WHEN user_agent ILIKE '%bluemail/%' THEN 'BlueMail'
        WHEN user_agent ILIKE '%superhuman%' THEN 'Superhuman'
        ELSE 'Outlook' END,
    client_type = 'app', browser = '', browser_version = '',
    os = CASE WHEN os = '' AND (user_agent ILIKE '%microsoft outlook%' OR user_agent ILIKE '%ms-office%' OR user_agent ILIKE '%msoffice%') THEN 'Windows' ELSE os END,
    device_type = CASE WHEN device_type IN ('', 'unknown') AND NOT user_agent ILIKE '%outlook-ios%' AND NOT user_agent ILIKE '%outlook-android%' AND NOT user_agent ILIKE '%bluemail/%' AND NOT user_agent ILIKE '%superhuman%' THEN 'desktop' ELSE device_type END
WHERE client_type = '' AND NOT device_hidden
  AND (user_agent ILIKE '%microsoft outlook%' OR user_agent ILIKE '%ms-office%' OR user_agent ILIKE '%msoffice%'
       OR user_agent ILIKE '%macoutlook%' OR user_agent ILIKE '%outlook-ios%' OR user_agent ILIKE '%outlook-android%'
       OR user_agent ILIKE '%thunderbird/%' OR user_agent ILIKE '%em client%' OR user_agent ILIKE '%mailbird/%'
       OR user_agent ILIKE '%mailspring/%' OR user_agent ILIKE '%bluemail/%' OR user_agent ILIKE '%superhuman%');

-- Mail on an iPhone or iPad: iOS WebKit with the build token and nothing a
-- browser or another app adds.
UPDATE email_opens
SET client = 'Apple Mail', client_type = 'app', browser = '', browser_version = ''
WHERE client_type = '' AND NOT device_hidden
  AND user_agent ~* '\((iphone|ipad);'
  AND user_agent ILIKE '%applewebkit/%' AND user_agent ILIKE '%mobile/%'
  AND user_agent !~* '(safari/|crios|fxios|edgios|opios|gsa/|fban|fbav|instagram|line/|micromessenger|twitter|linkedinapp|snapchat|pinterest|duckduckgo)';

-- An Android app's embedded WebView: a mail app, unnamed.
UPDATE email_opens
SET client_type = 'app', browser = '', browser_version = ''
WHERE client_type = '' AND NOT device_hidden AND client = ''
  AND user_agent ILIKE '%android%' AND user_agent ILIKE '%; wv)%';

-- A full browser is webmail read in a tab.
UPDATE email_opens
SET client_type = 'webmail'
WHERE client_type = '' AND NOT device_hidden AND client = ''
  AND user_agent ILIKE 'mozilla/5.0%'
  AND browser IN ('Chrome', 'Firefox', 'Safari', 'Edge', 'Opera', 'Vivaldi', 'Samsung Browser', 'Mobile Safari');

-- The device type no longer carries an "unknown" placeholder.
UPDATE email_opens SET device_type = '' WHERE device_type = 'unknown';
UPDATE email_link_clicks SET device_type = '' WHERE device_type = 'unknown';
