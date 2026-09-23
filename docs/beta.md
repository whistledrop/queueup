# Running the free beta

QueueUp is handed out free, in exchange for people saying how it went. This is
the operator's side of that.

## Where everything is

Open `/admin` on the website and paste the admin token (the
`QUEUEUP_ADMIN_TOKEN` secret on Fly). From top to bottom it answers:

1. **How joins went**, last 24 hours and last 7 days: got in, cancelled,
   failed, and failures broken down by reason. On force wipe day this is the
   number to watch.
2. **Failed joins**, with the email of whoever it happened to, so you can ask
   them about it.
3. **Feedback and problem reports.** Typed notes come from the site's feedback
   page. Reports come from the tray icon ("Send a problem report to
   QueueUp") and contain the agent log and the end of the Rust log. Tap
   **Read report** to open one. Delete reports once they have been dealt with:
   the privacy page promises that, and they contain Steam IDs.
4. **Accounts**, with two buttons each (below).

## Somebody forgot their password

There is no emailed reset yet (it needs an email provider). Instead: find them
under Accounts, press **Temporary password**, and send them what it shows over
whatever channel you are already talking on (a TikTok DM, say). It is shown
once. Their old password stops working and they are signed out everywhere.
Tell them to use it to sign in.

Only do this for somebody you are confident is the account's owner: anyone who
knows their email address can claim to have forgotten a password.

## Somebody asks to be deleted

The privacy page tells people to ask from the feedback page while signed in,
which is how you know the request is really theirs. Press **Erase** on their
account and type their email to confirm. Everything goes: PC, joins, schedules,
saved servers, feedback, reports. It cannot be undone. Their PC's agent is
disconnected and unlinks itself.

## Limits that exist to protect the beta

- Five new accounts per internet address per hour.
- Server search answers are reused for a minute, so a video's worth of people
  typing does not burn through the daily Steam API allowance.
- Problem reports are capped at 2 MB.

## Ending the beta

When it is time to charge, both of these change together:

1. `BETA = false` in `web/lib/pricing.ts` (removes the beta labels and puts the
   price back on the landing page).
2. `fly secrets set QUEUEUP_BILLING=on -a queueup-relay` (turns the
   subscription gate on). Stripe has to be connected first.

Before the beta ends, delete any problem reports that are left: the privacy
page says they go no later than that.

## Still missing before a big launch

- **Code signing.** Two separate problems, and the second is worse.
  Every tester sees the "Windows protected your PC" warning; the dashboard
  walks them through it, but it costs signups. On a NEW Windows 11 PC, which is
  what a new gaming laptop is, **Smart App Control blocks QueueUp outright**,
  with no "run anyway": it refuses anything not signed by a known publisher.
  Those people have to turn Smart App Control off before they can run QueueUp
  at all (Windows Security, App & browser control), and help says so. That is a
  hard stop for some share of exactly the audience TikTok sends. Signing moves
  from "nice to have" to the thing most likely to cap the beta.
- **Email.** Needed for a real password reset.
- **A contact address on the privacy page.** A privacy notice should say who
  is responsible for the data and how to reach them. Logan's personal name
  stays off it: it will say "QueueUp" with a contact address on QueueUp's own
  domain (hello@queueuprust.com) once the domain exists. Until then it points
  people at the feedback page.
