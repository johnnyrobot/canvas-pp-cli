---
name: pp-canvas
description: "The whole Canvas REST API in one Go binary — plus a local store, offline search, and cross-resource commands (roster, at-risk, standings) that no single Canvas endpoint returns. Trigger phrases: `canvas roster for course`, `who is missing work in canvas`, `what do I still need to grade in canvas`, `canvas course standings this term`, `audit canvas enrollments`, `use canvas-pp-cli`, `run canvas cli`."
author: "johnnyrobot"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - canvas-pp-cli
    install:
      - kind: go
        bins: [canvas-pp-cli]
        module: github.com/johnnyrobot/canvas-pp-cli/cmd/canvas-pp-cli
---

# Canvas LMS — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `canvas-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install with Go (requires Go 1.26.5 or newer). This installs into `$GOPATH/bin` (default `$HOME/go/bin`), so that directory must be on `$PATH`:
   ```bash
   go install github.com/johnnyrobot/canvas-pp-cli/cmd/canvas-pp-cli@latest
   ```
2. Verify: `canvas-pp-cli --version`
3. Ensure the reported install directory is on `$PATH` for the agent/runtime that will invoke this skill.

If Go is unavailable, download a pre-built binary from https://github.com/johnnyrobot/canvas-pp-cli/releases/latest, put it on `$PATH`, and verify the same way. On macOS clear the quarantine flag first: `xattr -d com.apple.quarantine canvas-pp-cli`.

If `--version` reports "command not found" after install, the runtime cannot see the binary directory on `$PATH`. Do not proceed with skill commands until verification succeeds.

canvas-pp-cli mirrors the full Canvas LMS REST surface as typed, agent-native commands and keeps a local SQLite mirror you can join and search offline. Beyond the endpoint wrappers every SDK ships, it adds joins Canvas never returns directly: 'roster' fuses enrollments, sections, and grades; 'at-risk' and 'to-grade' aggregate submissions across all your courses; 'standings' rolls grades up by term; 'audit-enrollments' finds account anomalies.

## When to Use This CLI

Use canvas-pp-cli when an agent or admin needs to read or change Canvas LMS data from the terminal at scale: pulling rosters and grades, finding missing or ungraded work across many courses, running term-level standings, or driving any of the 1,000+ REST endpoints non-interactively with stable JSON. It is the right tool when the task spans more than one course or joins more than one Canvas resource.

## Anti-triggers

Do not use this CLI for:
- Do not use this CLI for LTI 1.3 tool launches or deep-linking; those are browser/OIDC flows, not REST calls.
- Do not use it to render the Canvas web UI or student-facing pages; it is an API client, not a browser.
- Do not use it for Canvas Data 2 / DAP bulk warehouse extracts; that is a separate batch product, not the /api/v1 REST API.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-resource joins

- **`roster`** — One table of every student in a course with section, login/SIS ID, role, current and final score, and missing-work count.

  _Reach for this instead of paging three endpoints when you need a complete per-student course view in one call._

  ```bash
  canvas-pp-cli roster 12345 --agent
  ```
- **`at-risk`** — Ranked list of students with missing, late, or unsubmitted work across one or all of an instructor's courses, time-windowed.

  _Use this for early-alert outreach; it is the one call that answers 'who is falling behind right now'._

  ```bash
  canvas-pp-cli at-risk --all-my-courses --since 14d --agent
  ```
- **`to-grade`** — A single oldest-first queue of every submitted-but-ungraded item across the instructor's courses, with student, assignment, and points.

  _Pick this to plan a grading session across courses without opening each gradebook._

  ```bash
  canvas-pp-cli to-grade --all-my-courses --sort oldest --agent
  ```

### Local snapshots that compound

- **`since`** — A 'what changed' digest of new submissions, new announcements, and new enrollments since a time window.

  _Use this for a standup-style digest of course activity over a window the API cannot query directly._

  ```bash
  canvas-pp-cli since 24h --course 12345 --agent
  ```

### Admin rollups

- **`standings`** — Grade-distribution and pass/fail/DFW rollups computed locally across every course in a term, drillable by section or course.

  _Reach for this for term-level reporting that joins enrollments, submissions, and courses no single endpoint combines._

  ```bash
  canvas-pp-cli standings --term 7 --by course --agent
  ```
- **`audit-enrollments`** — Flags enrollment anomalies in an account: students with no submissions, teacher-less courses, and concluded-but-active users.

  _Use this for compliance and cleanup sweeps before a term rolls over._

  ```bash
  canvas-pp-cli audit-enrollments --account 1 --orphans --agent
  ```

## Command Reference

**access_tokens** — {

- `canvas-pp-cli access-tokens create` — Create an access token
- `canvas-pp-cli access-tokens destroy` — Delete an access token
- `canvas-pp-cli access-tokens show` — Show an access token
- `canvas-pp-cli access-tokens update` — Update an access token
- `canvas-pp-cli access-tokens user-generated-tokens` — List access tokens for a user

**accessibility_course_scans** — [AccessibilityCourseScansController#create](https://github.com/instructure/canvas-lms/blob/master/app/controllers/accessibility_course_scans_controller.rb)

- `canvas-pp-cli accessibility-course-scans <user_id>` — Trigger accessibility course scan

**accessibility_course_statistics** — // Per-course accessibility issue counts for a user's active teacher/designer

- `canvas-pp-cli accessibility-course-statistics <user_id>` — List accessibility course statistics

**account_calendars** — API for viewing and toggling settings of account calendars.

- `canvas-pp-cli account-calendars all-calendars` — List all account calendars
- `canvas-pp-cli account-calendars bulk-update` — Update several calendars
- `canvas-pp-cli account-calendars index` — List available account calendars
- `canvas-pp-cli account-calendars show` — Get a single account calendar
- `canvas-pp-cli account-calendars update` — Update a calendar
- `canvas-pp-cli account-calendars visible-calendars-count` — Count of all visible account calendars

**account_domain_lookups** — 

- `canvas-pp-cli account-domain-lookups` — Search account domains

**account_notifications** — API for account notifications.

- `canvas-pp-cli account-notifications create` — Create a global notification
- `canvas-pp-cli account-notifications show` — Show a global notification
- `canvas-pp-cli account-notifications update` — Update a global notification
- `canvas-pp-cli account-notifications user-close-notification` — Close notification for user. Destroy notification for admin
- `canvas-pp-cli account-notifications user-index` — Index of active global notification for the user

**account_reports** — API for accessing account reports.

- `canvas-pp-cli account-reports abort` — Abort a Report
- `canvas-pp-cli account-reports available-reports` — List Available Reports
- `canvas-pp-cli account-reports create` — Start a Report
- `canvas-pp-cli account-reports destroy` — Delete a Report
- `canvas-pp-cli account-reports index` — Index of Reports
- `canvas-pp-cli account-reports show` — Status of a Report

**accounts** — API for accessing account data.

- `canvas-pp-cli accounts course-accounts` — List accounts for course admins
- `canvas-pp-cli accounts course-creation-accounts` — Get accounts that users can create courses in
- `canvas-pp-cli accounts courses-api` — List active courses in an account
- `canvas-pp-cli accounts create` — Create a new sub-account
- `canvas-pp-cli accounts destroy` — Delete a sub-account
- `canvas-pp-cli accounts environment` — List environment settings
- `canvas-pp-cli accounts help-links` — Get help links
- `canvas-pp-cli accounts horizon-accounts` — List horizon accounts
- `canvas-pp-cli accounts index` — List accounts
- `canvas-pp-cli accounts manageable-accounts` — Get accounts that admins can manage
- `canvas-pp-cli accounts manually-created-courses-account` — Get the manually-created courses sub-account for the domain root account
- `canvas-pp-cli accounts permissions` — Permissions
- `canvas-pp-cli accounts remove-user` — Delete a user from the root account
- `canvas-pp-cli accounts remove-users` — Delete multiple users from the root account
- `canvas-pp-cli accounts restore-user` — Restore a deleted user from a root account
- `canvas-pp-cli accounts show` — Get a single account
- `canvas-pp-cli accounts show-settings` — Settings
- `canvas-pp-cli accounts sub-accounts` — Get the sub-accounts of an account
- `canvas-pp-cli accounts terms-of-service` — Get the Terms of Service
- `canvas-pp-cli accounts update` — Update an account
- `canvas-pp-cli accounts update-users` — Update multiple users

**admins** — Manage account role assignments

- `canvas-pp-cli admins create` — Make an account admin
- `canvas-pp-cli admins destroy` — Remove account admin
- `canvas-pp-cli admins index` — List account admins
- `canvas-pp-cli admins self-roles` — List my admin roles

**ai_conversations** — API for managing conversations with AI Experiences.

- `canvas-pp-cli ai-conversations active-conversation` — Get active conversation
- `canvas-pp-cli ai-conversations create` — Create AI conversation
- `canvas-pp-cli ai-conversations create-feedback` — Create feedback on a conversation message
- `canvas-pp-cli ai-conversations delete-feedback` — Delete feedback on a conversation message
- `canvas-pp-cli ai-conversations destroy` — Delete AI conversation
- `canvas-pp-cli ai-conversations evaluation` — Get conversation evaluation
- `canvas-pp-cli ai-conversations post-message` — Post message to conversation
- `canvas-pp-cli ai-conversations show` — Show conversation

**ai_experiences** — API for creating, accessing and updating AI Experiences. AI Experiences are used to create interactive AI-powered learning scenarios within courses.

- `canvas-pp-cli ai-experiences ai-conversation-show` — Show student AI conversation
- `canvas-pp-cli ai-experiences ai-conversations-index` — List student AI conversations
- `canvas-pp-cli ai-experiences create` — Create an AI experience
- `canvas-pp-cli ai-experiences destroy` — Delete an AI experience
- `canvas-pp-cli ai-experiences edit` — Show edit AI experience form
- `canvas-pp-cli ai-experiences index` — List AI experiences
- `canvas-pp-cli ai-experiences new` — Show new AI experience form
- `canvas-pp-cli ai-experiences show` — Show an AI experience
- `canvas-pp-cli ai-experiences update` — Update an AI experience

**analytics_resource** — API for retrieving the data exposed in Canvas Analytics

- `canvas-pp-cli analytics-resource course-assignments` — Get course-level assignment data
- `canvas-pp-cli analytics-resource course-participation` — Get course-level participation data
- `canvas-pp-cli analytics-resource course-student-summaries` — Get course-level student summary data
- `canvas-pp-cli analytics-resource department-grades` — Get department-level grade data
- `canvas-pp-cli analytics-resource department-grades-2` — Get department-level grade data
- `canvas-pp-cli analytics-resource department-grades-3` — Get department-level grade data
- `canvas-pp-cli analytics-resource department-participation` — Get department-level participation data
- `canvas-pp-cli analytics-resource department-participation-2` — Get department-level participation data
- `canvas-pp-cli analytics-resource department-participation-3` — Get department-level participation data
- `canvas-pp-cli analytics-resource department-statistics` — Get department-level statistics
- `canvas-pp-cli analytics-resource department-statistics-2` — Get department-level statistics
- `canvas-pp-cli analytics-resource department-statistics-3` — Get department-level statistics
- `canvas-pp-cli analytics-resource department-statistics-by-subaccount` — Get department-level statistics, broken down by subaccount
- `canvas-pp-cli analytics-resource department-statistics-by-subaccount-2` — Get department-level statistics, broken down by subaccount
- `canvas-pp-cli analytics-resource department-statistics-by-subaccount-3` — Get department-level statistics, broken down by subaccount
- `canvas-pp-cli analytics-resource student-in-course-assignments` — Get user-in-a-course-level assignment data
- `canvas-pp-cli analytics-resource student-in-course-messaging` — Get user-in-a-course-level messaging data
- `canvas-pp-cli analytics-resource student-in-course-participation` — Get user-in-a-course-level participation data

**announcement_external_feeds** — External feeds represent RSS feeds that can be attached to a Course or Group, in order to automatically create announcements for each new item in the feed.

- `canvas-pp-cli announcement-external-feeds create` — Create an external feed
- `canvas-pp-cli announcement-external-feeds create-2` — Create an external feed
- `canvas-pp-cli announcement-external-feeds destroy` — Delete an external feed
- `canvas-pp-cli announcement-external-feeds destroy-2` — Delete an external feed
- `canvas-pp-cli announcement-external-feeds index` — List external feeds
- `canvas-pp-cli announcement-external-feeds index-2` — List external feeds

**announcements** — API for retrieving announcements. This API is Announcement-specific. See also the Discussion Topics API, which operates on Announcements also.

- `canvas-pp-cli announcements` — List announcements

**api_token_scopes** — {% hint style="warning" %}

- `canvas-pp-cli api-token-scopes <account_id>` — List scopes

**appointment_groups** — API for creating, accessing and updating appointment groups. Appointment groups provide a way of creating a bundle of time slots that users can sign up for (e.g. "Office Hours" or "Meet with professor about Final Project"). Both time slots 

- `canvas-pp-cli appointment-groups create` — Create an appointment group
- `canvas-pp-cli appointment-groups destroy` — Delete an appointment group
- `canvas-pp-cli appointment-groups groups` — List student group participants
- `canvas-pp-cli appointment-groups index` — List appointment groups
- `canvas-pp-cli appointment-groups next-appointment` — Get next appointment
- `canvas-pp-cli appointment-groups show` — Get a single appointment group
- `canvas-pp-cli appointment-groups update` — Update an appointment group
- `canvas-pp-cli appointment-groups users` — List user participants

**assessment_question_banks** — {

- `canvas-pp-cli assessment-question-banks index` — List question banks
- `canvas-pp-cli assessment-question-banks questions` — List assessment questions for a question bank
- `canvas-pp-cli assessment-question-banks show` — Get a single question bank

**assignment_extensions** — API for setting extensions on student assignment submissions. These cannot be set for discussion assignments or quizzes. For quizzes, use [Quiz Extensions](/services/canvas/resources/quiz_extensions) instead.

- `canvas-pp-cli assignment-extensions <course_id> <assignment_id>` — Set extensions for student assignment submissions

**assignment_groups** — API for accessing Assignment Group and Assignment information.

- `canvas-pp-cli assignment-groups create` — Create an Assignment Group
- `canvas-pp-cli assignment-groups destroy` — Destroy an Assignment Group
- `canvas-pp-cli assignment-groups index` — List assignment groups
- `canvas-pp-cli assignment-groups show` — Get an Assignment Group
- `canvas-pp-cli assignment-groups update` — Edit an Assignment Group

**assignments** — API for accessing assignment information.

- `canvas-pp-cli assignments batch-create` — Batch create overrides in a course
- `canvas-pp-cli assignments batch-retrieve` — Batch retrieve overrides in a course
- `canvas-pp-cli assignments batch-update` — Batch update overrides in a course
- `canvas-pp-cli assignments bulk-update` — Bulk update assignment dates
- `canvas-pp-cli assignments create` — Create an assignment
- `canvas-pp-cli assignments create-2` — Create an assignment override
- `canvas-pp-cli assignments destroy` — Delete an assignment
- `canvas-pp-cli assignments destroy-2` — Delete an assignment override
- `canvas-pp-cli assignments duplicate` — Duplicate assignment
- `canvas-pp-cli assignments group-alias` — Redirect to the assignment override for a group
- `canvas-pp-cli assignments index` — List assignments
- `canvas-pp-cli assignments index-2` — List assignments
- `canvas-pp-cli assignments index-3` — List assignment overrides
- `canvas-pp-cli assignments section-alias` — Redirect to the assignment override for a section
- `canvas-pp-cli assignments show` — Get a single assignment
- `canvas-pp-cli assignments show-2` — Get a single assignment override
- `canvas-pp-cli assignments student-group-members` — List group members for a student on an assignment
- `canvas-pp-cli assignments update` — Edit an assignment
- `canvas-pp-cli assignments update-2` — Update an assignment override
- `canvas-pp-cli assignments user-index` — List assignments for user

**authentication_providers** — {

- `canvas-pp-cli authentication-providers create` — Add authentication provider
- `canvas-pp-cli authentication-providers destroy` — Delete authentication provider
- `canvas-pp-cli authentication-providers force-password-reset` — Force password reset
- `canvas-pp-cli authentication-providers index` — List authentication providers
- `canvas-pp-cli authentication-providers restore` — Restore a deleted authentication provider
- `canvas-pp-cli authentication-providers show` — Get authentication provider
- `canvas-pp-cli authentication-providers show-sso-settings` — Show account auth settings
- `canvas-pp-cli authentication-providers update` — Update authentication provider
- `canvas-pp-cli authentication-providers update-sso-settings` — Update account auth settings

**authentications_log** — Query audit log of authentication events (logins and logouts).

- `canvas-pp-cli authentications-log for-account` — Query by account.
- `canvas-pp-cli authentications-log for-login` — Query by login.
- `canvas-pp-cli authentications-log for-user` — Query by user.

**blackout_dates** — API for accessing blackout date information.

- `canvas-pp-cli blackout-dates bulk-update` — Update a list of Blackout Dates
- `canvas-pp-cli blackout-dates create` — Create Blackout Date
- `canvas-pp-cli blackout-dates create-2` — Create Blackout Date
- `canvas-pp-cli blackout-dates destroy` — Delete Blackout Date
- `canvas-pp-cli blackout-dates destroy-2` — Delete Blackout Date
- `canvas-pp-cli blackout-dates index` — List blackout dates
- `canvas-pp-cli blackout-dates index-2` — List blackout dates
- `canvas-pp-cli blackout-dates new` — New Blackout Date
- `canvas-pp-cli blackout-dates new-2` — New Blackout Date
- `canvas-pp-cli blackout-dates show` — Get a single blackout date
- `canvas-pp-cli blackout-dates show-2` — Get a single blackout date
- `canvas-pp-cli blackout-dates update` — Update Blackout Date
- `canvas-pp-cli blackout-dates update-2` — Update Blackout Date

**blockeditortemplate** — Block Editor Templates are pre-build templates that can be used to create pages. The BlockEditorTemplate API allows you to create, retrieve, update, and delete templates.

- `canvas-pp-cli blockeditortemplate <course_id>` — List block templates

**blueprint_courses** — Configure blueprint courses

- `canvas-pp-cli blueprint-courses get-associated-courses` — Returns a list of courses that are configured to receive updates from this blueprint
- `canvas-pp-cli blueprint-courses get-blueprint-subscriptions` — Returns a list of blueprint subscriptions for the given course. (Currently a course may have no more than one.)
- `canvas-pp-cli blueprint-courses get-blueprint-templates` — Using 'default' as the template_id should suffice for the current implmentation (as there should be only one template pe
- `canvas-pp-cli blueprint-courses get-details` — Show the changes that were propagated in a blueprint migration. This endpoint can be called on a blueprint course. See a
- `canvas-pp-cli blueprint-courses get-details-2` — Show the changes that were propagated to a course associated with a blueprint. See also [the blueprint course side](#met
- `canvas-pp-cli blueprint-courses get-migrations` — Shows a paginated list of migrations for the template, starting with the most recent. This endpoint can be called on a b
- `canvas-pp-cli blueprint-courses get-migrations-2` — Shows the status of a migration. This endpoint can be called on a blueprint course. See also [the associated course side
- `canvas-pp-cli blueprint-courses get-migrations-3` — Shows a paginated list of migrations imported into a course associated with a blueprint, starting with the most recent.
- `canvas-pp-cli blueprint-courses get-migrations-4` — Shows the status of an import into a course associated with a blueprint. See also [the blueprint course side](#method.ma
- `canvas-pp-cli blueprint-courses get-unsynced-changes` — Retrieve a list of learning objects that have changed since the last blueprint sync operation. If no syncs have been com
- `canvas-pp-cli blueprint-courses post-migrations` — Begins a migration to push recently updated content to all associated courses. Only one migration can be running at a ti
- `canvas-pp-cli blueprint-courses put-restrict-item` — If a blueprint course object is restricted, editing will be limited for copies in associated courses.
- `canvas-pp-cli blueprint-courses put-update-associations` — Send a list of course ids to add or remove new associations for the template. Cannot add courses that do not belong to t

**bookmarks** — {

- `canvas-pp-cli bookmarks delete-bookmarks` — Deletes a bookmark
- `canvas-pp-cli bookmarks get-bookmarks` — Returns the paginated list of bookmarks.
- `canvas-pp-cli bookmarks get-bookmarks-2` — Returns the details for a bookmark.
- `canvas-pp-cli bookmarks post-bookmarks` — Creates a bookmark.
- `canvas-pp-cli bookmarks put-bookmarks` — Updates a bookmark

**brand_configs** — [BrandConfigsApiController#show](https://github.com/instructure/canvas-lms/blob/master/app/controllers/brand_configs_api_controller.rb)

- `canvas-pp-cli brand-configs show` — Get the brand config variables that should be used for this domain
- `canvas-pp-cli brand-configs show-context` — Get the brand config variables for a sub-account or course
- `canvas-pp-cli brand-configs show-context-2` — Get the brand config variables for a sub-account or course

**calendar_events** — API for creating, accessing and updating calendar events.

- `canvas-pp-cli calendar-events create` — Create a calendar event
- `canvas-pp-cli calendar-events destroy` — Delete a calendar event
- `canvas-pp-cli calendar-events get-course-timetable` — Get course timetable
- `canvas-pp-cli calendar-events index` — List calendar events
- `canvas-pp-cli calendar-events reserve` — Reserve a time slot
- `canvas-pp-cli calendar-events reserve-2` — Reserve a time slot
- `canvas-pp-cli calendar-events save-enabled-account-calendars` — Save enabled account calendars
- `canvas-pp-cli calendar-events set-course-timetable` — Set a course timetable
- `canvas-pp-cli calendar-events set-course-timetable-events` — Create or update events directly for a course timetable
- `canvas-pp-cli calendar-events show` — Get a single calendar event or assignment
- `canvas-pp-cli calendar-events update` — Update a calendar event
- `canvas-pp-cli calendar-events user-index` — List calendar events for a user

**canvas_career_experiences** — API for managing user career experience and role preferences in Canvas.

- `canvas-pp-cli canvas-career-experiences enabled` — Check if Canvas Career is enabled
- `canvas-pp-cli canvas-career-experiences experience-summary` — Get current and available experiences
- `canvas-pp-cli canvas-career-experiences switch-experience` — Switch experience
- `canvas-pp-cli canvas-career-experiences switch-role` — Switch role

**collaborations** — API for accessing course and group collaboration information.

- `canvas-pp-cli collaborations api-index` — List collaborations
- `canvas-pp-cli collaborations api-index-2` — List collaborations
- `canvas-pp-cli collaborations members` — List members of a collaboration.
- `canvas-pp-cli collaborations potential-collaborators` — List potential members
- `canvas-pp-cli collaborations potential-collaborators-2` — List potential members

**commmessages** — API for accessing the messages (emails, sms, etc) that have been sent to a user.

- `canvas-pp-cli commmessages` — List of CommMessages for a user

**communication_channels** — API for accessing users' email and SMS communication channels.

- `canvas-pp-cli communication-channels create` — Create a communication channel
- `canvas-pp-cli communication-channels delete-push-token` — Delete a push notification endpoint
- `canvas-pp-cli communication-channels destroy` — Delete a communication channel
- `canvas-pp-cli communication-channels destroy-2` — Delete a communication channel
- `canvas-pp-cli communication-channels index` — List user communication channels

**conferences** — API for accessing information on conferences.

- `canvas-pp-cli conferences for-user` — List conferences for the current user
- `canvas-pp-cli conferences index` — List conferences
- `canvas-pp-cli conferences index-2` — List conferences

**content_exports** — API for exporting courses and course content

- `canvas-pp-cli content-exports create` — Export content
- `canvas-pp-cli content-exports create-2` — Export content
- `canvas-pp-cli content-exports create-3` — Export content
- `canvas-pp-cli content-exports index` — List content exports
- `canvas-pp-cli content-exports index-2` — List content exports
- `canvas-pp-cli content-exports index-3` — List content exports
- `canvas-pp-cli content-exports show` — Show content export
- `canvas-pp-cli content-exports show-2` — Show content export
- `canvas-pp-cli content-exports show-3` — Show content export

**content_migrations** — API for accessing content migrations and migration issues

- `canvas-pp-cli content-migrations asset-id-mapping` — Get asset id mapping
- `canvas-pp-cli content-migrations available-migrators` — List Migration Systems
- `canvas-pp-cli content-migrations available-migrators-2` — List Migration Systems
- `canvas-pp-cli content-migrations available-migrators-3` — List Migration Systems
- `canvas-pp-cli content-migrations available-migrators-4` — List Migration Systems
- `canvas-pp-cli content-migrations content-list` — List items for selective import
- `canvas-pp-cli content-migrations content-list-2` — List items for selective import
- `canvas-pp-cli content-migrations content-list-3` — List items for selective import
- `canvas-pp-cli content-migrations content-list-4` — List items for selective import
- `canvas-pp-cli content-migrations create` — Create a content migration
- `canvas-pp-cli content-migrations create-2` — Create a content migration
- `canvas-pp-cli content-migrations create-3` — Create a content migration
- `canvas-pp-cli content-migrations create-4` — Create a content migration
- `canvas-pp-cli content-migrations index` — List migration issues
- `canvas-pp-cli content-migrations index-2` — List migration issues
- `canvas-pp-cli content-migrations index-3` — List migration issues
- `canvas-pp-cli content-migrations index-4` — List migration issues
- `canvas-pp-cli content-migrations index-5` — List content migrations
- `canvas-pp-cli content-migrations index-6` — List content migrations
- `canvas-pp-cli content-migrations index-7` — List content migrations
- `canvas-pp-cli content-migrations index-8` — List content migrations
- `canvas-pp-cli content-migrations show` — Get a migration issue
- `canvas-pp-cli content-migrations show-2` — Get a migration issue
- `canvas-pp-cli content-migrations show-3` — Get a migration issue
- `canvas-pp-cli content-migrations show-4` — Get a migration issue
- `canvas-pp-cli content-migrations show-5` — Get a content migration
- `canvas-pp-cli content-migrations show-6` — Get a content migration
- `canvas-pp-cli content-migrations show-7` — Get a content migration
- `canvas-pp-cli content-migrations show-8` — Get a content migration
- `canvas-pp-cli content-migrations update` — Update a migration issue
- `canvas-pp-cli content-migrations update-2` — Update a migration issue
- `canvas-pp-cli content-migrations update-3` — Update a migration issue
- `canvas-pp-cli content-migrations update-4` — Update a migration issue
- `canvas-pp-cli content-migrations update-5` — Update a content migration
- `canvas-pp-cli content-migrations update-6` — Update a content migration
- `canvas-pp-cli content-migrations update-7` — Update a content migration
- `canvas-pp-cli content-migrations update-8` — Update a content migration

**content_security_policy_settings** — {% hint style="warning" %}

- `canvas-pp-cli content-security-policy-settings add-domain` — Add an allowed domain to account
- `canvas-pp-cli content-security-policy-settings add-multiple-domains` — Add multiple allowed domains to an account
- `canvas-pp-cli content-security-policy-settings get-csp-settings` — Get current settings for account or course
- `canvas-pp-cli content-security-policy-settings get-csp-settings-2` — Get current settings for account or course
- `canvas-pp-cli content-security-policy-settings remove-domain` — Remove a domain from account
- `canvas-pp-cli content-security-policy-settings set-csp-lock` — Lock or unlock current CSP settings for sub-accounts and courses
- `canvas-pp-cli content-security-policy-settings set-csp-setting` — Enable, disable, or clear explicit CSP setting
- `canvas-pp-cli content-security-policy-settings set-csp-setting-2` — Enable, disable, or clear explicit CSP setting

**content_shares** — API for creating, accessing and updating Content Sharing. Content shares are used to share content directly between users.

- `canvas-pp-cli content-shares add-users` — Add users to content share
- `canvas-pp-cli content-shares create` — Create a content share
- `canvas-pp-cli content-shares destroy` — Remove content share
- `canvas-pp-cli content-shares index` — List content shares
- `canvas-pp-cli content-shares index-2` — List content shares
- `canvas-pp-cli content-shares show` — Get content share
- `canvas-pp-cli content-shares unread-count` — Get unread shares count
- `canvas-pp-cli content-shares update` — Update a content share

**conversations** — API for creating, accessing and updating user conversations.

- `canvas-pp-cli conversations add-message` — Add a message
- `canvas-pp-cli conversations add-recipients` — Add recipients
- `canvas-pp-cli conversations batch-update` — Batch update conversations
- `canvas-pp-cli conversations batches` — Get running batches
- `canvas-pp-cli conversations create` — Create a conversation
- `canvas-pp-cli conversations destroy` — Delete a conversation
- `canvas-pp-cli conversations find-recipients` — Find recipients
- `canvas-pp-cli conversations index` — List conversations
- `canvas-pp-cli conversations mark-all-as-read` — Mark all as read
- `canvas-pp-cli conversations remove-messages` — Delete a message
- `canvas-pp-cli conversations show` — Get a single conversation
- `canvas-pp-cli conversations unread-count` — Unread count
- `canvas-pp-cli conversations update` — Edit a conversation

**course_audit_log** — Query audit log of course events.

- `canvas-pp-cli course-audit-log for-account` — Query by account.
- `canvas-pp-cli course-audit-log for-course` — Query by course.

**course_pace** — API for accessing and building Course Paces.

- `canvas-pp-cli course-pace api-show` — Show a Course pace
- `canvas-pp-cli course-pace create` — Create a Course pace
- `canvas-pp-cli course-pace destroy` — Delete a Course pace
- `canvas-pp-cli course-pace update` — Update a Course pace

**course_quiz_extensions** — API for setting extensions on student quiz submissions at the course level

- `canvas-pp-cli course-quiz-extensions <course_id>` — Responses

**course_reports** — API for accessing course reports.

- `canvas-pp-cli course-reports create` — Start a Report
- `canvas-pp-cli course-reports last` — Status of last Report
- `canvas-pp-cli course-reports show` — Status of a Report

**courses** — API for accessing course information.

- `canvas-pp-cli courses activity-stream` — Course activity stream
- `canvas-pp-cli courses activity-stream-summary` — Course activity stream summary
- `canvas-pp-cli courses api-settings` — Get course settings
- `canvas-pp-cli courses batch-update` — Update courses
- `canvas-pp-cli courses bulk-user-progress` — Get bulk user progress
- `canvas-pp-cli courses content-share-users` — Search for content share users
- `canvas-pp-cli courses copy-course-content` — Copy course content
- `canvas-pp-cli courses copy-course-status` — Get course copy status
- `canvas-pp-cli courses create` — Create a new course
- `canvas-pp-cli courses create-file` — Upload a file
- `canvas-pp-cli courses destroy` — Delete/Conclude a course
- `canvas-pp-cli courses dismiss-migration-limitation-msg` — Remove quiz migration alert
- `canvas-pp-cli courses effective-due-dates` — Get effective due dates
- `canvas-pp-cli courses index` — List your courses
- `canvas-pp-cli courses permissions` — Permissions
- `canvas-pp-cli courses preview-html` — Preview processed html
- `canvas-pp-cli courses recent-students` — List recently logged in students
- `canvas-pp-cli courses reset-content` — Reset a course
- `canvas-pp-cli courses restore-version` — Restore course syllabus version
- `canvas-pp-cli courses show` — Get a single course
- `canvas-pp-cli courses show-2` — Get a single course
- `canvas-pp-cli courses student-view-student` — Return test student for course
- `canvas-pp-cli courses students` — List students
- `canvas-pp-cli courses todo-items` — Course TODO items
- `canvas-pp-cli courses update` — Update a course
- `canvas-pp-cli courses update-settings` — Update course settings
- `canvas-pp-cli courses user` — Get single user
- `canvas-pp-cli courses user-index` — List courses for a user
- `canvas-pp-cli courses user-progress` — Get user progress
- `canvas-pp-cli courses users` — List users in course
- `canvas-pp-cli courses users-2` — List users in course

**custom_gradebook_columns** — API for adding additional columns to the gradebook. Custom gradebook columns will be displayed with the other frozen gradebook columns.

- `canvas-pp-cli custom-gradebook-columns bulk-update` — Bulk update column data
- `canvas-pp-cli custom-gradebook-columns create` — Create a custom gradebook column
- `canvas-pp-cli custom-gradebook-columns destroy` — Delete a custom gradebook column
- `canvas-pp-cli custom-gradebook-columns index` — List custom gradebook columns
- `canvas-pp-cli custom-gradebook-columns index-2` — List entries for a column
- `canvas-pp-cli custom-gradebook-columns reorder` — Reorder custom columns
- `canvas-pp-cli custom-gradebook-columns update` — Update a custom gradebook column
- `canvas-pp-cli custom-gradebook-columns update-2` — Update column data

**developer_key_account_bindings** — Developer key account bindings API for binding a developer key to a context and specifying a workflow state for that relationship.

- `canvas-pp-cli developer-key-account-bindings <account_id> <developer_key_id>` — Create a Developer Key Account Binding

**developer_keys** — Manage Canvas API Keys, used for OAuth access to this API. See [the OAuth access docs](/services/canvas/oauth2/file.oauth) for usage of these keys. Note that DeveloperKeys are also (currently) used for LTI 1.3 registration and OIDC access, 

- `canvas-pp-cli developer-keys create` — Create a Developer Key
- `canvas-pp-cli developer-keys destroy` — Delete a Developer Key
- `canvas-pp-cli developer-keys index` — List Developer Keys
- `canvas-pp-cli developer-keys regenerate-secret` — Regenerate Developer Key Secret
- `canvas-pp-cli developer-keys update` — Update a Developer Key

**discovery_pages** — // Configuration for the login discovery page

- `canvas-pp-cli discovery-pages show` — Get Discovery Page
- `canvas-pp-cli discovery-pages token` — Generate Discovery Page Preview Token
- `canvas-pp-cli discovery-pages upsert` — Update Discovery Page

**discussion_topics** — API for accessing and participating in discussion topics in groups and courses.

- `canvas-pp-cli discussion-topics add-entry` — Post an entry
- `canvas-pp-cli discussion-topics add-entry-2` — Post an entry
- `canvas-pp-cli discussion-topics add-reply` — Post a reply
- `canvas-pp-cli discussion-topics add-reply-2` — Post a reply
- `canvas-pp-cli discussion-topics create` — Create a new discussion topic
- `canvas-pp-cli discussion-topics create-2` — Create a new discussion topic
- `canvas-pp-cli discussion-topics destroy` — Delete a topic
- `canvas-pp-cli discussion-topics destroy-2` — Delete a topic
- `canvas-pp-cli discussion-topics destroy-3` — Delete an entry
- `canvas-pp-cli discussion-topics destroy-4` — Delete an entry
- `canvas-pp-cli discussion-topics disable-summary` — Disable summary
- `canvas-pp-cli discussion-topics disable-summary-2` — Disable summary
- `canvas-pp-cli discussion-topics duplicate` — Duplicate discussion topic
- `canvas-pp-cli discussion-topics duplicate-2` — Duplicate discussion topic
- `canvas-pp-cli discussion-topics entries` — List topic entries
- `canvas-pp-cli discussion-topics entries-2` — List topic entries
- `canvas-pp-cli discussion-topics entry-list` — List entries
- `canvas-pp-cli discussion-topics entry-list-2` — List entries
- `canvas-pp-cli discussion-topics find-or-create-summary` — Find or Create Summary
- `canvas-pp-cli discussion-topics find-or-create-summary-2` — Find or Create Summary
- `canvas-pp-cli discussion-topics find-summary` — Find Last Summary
- `canvas-pp-cli discussion-topics find-summary-2` — Find Last Summary
- `canvas-pp-cli discussion-topics index` — List discussion topics
- `canvas-pp-cli discussion-topics index-2` — List discussion topics
- `canvas-pp-cli discussion-topics mark-all-read` — Mark all entries as read
- `canvas-pp-cli discussion-topics mark-all-read-2` — Mark all entries as read
- `canvas-pp-cli discussion-topics mark-all-topic-read` — Mark all topic as read
- `canvas-pp-cli discussion-topics mark-all-topic-read-2` — Mark all topic as read
- `canvas-pp-cli discussion-topics mark-all-unread` — Mark all entries as unread
- `canvas-pp-cli discussion-topics mark-all-unread-2` — Mark all entries as unread
- `canvas-pp-cli discussion-topics mark-entry-read` — Mark entry as read
- `canvas-pp-cli discussion-topics mark-entry-read-2` — Mark entry as read
- `canvas-pp-cli discussion-topics mark-entry-unread` — Mark entry as unread
- `canvas-pp-cli discussion-topics mark-entry-unread-2` — Mark entry as unread
- `canvas-pp-cli discussion-topics mark-topic-read` — Mark topic as read
- `canvas-pp-cli discussion-topics mark-topic-read-2` — Mark topic as read
- `canvas-pp-cli discussion-topics mark-topic-unread` — Mark topic as unread
- `canvas-pp-cli discussion-topics mark-topic-unread-2` — Mark topic as unread
- `canvas-pp-cli discussion-topics rate-entry` — Rate entry
- `canvas-pp-cli discussion-topics rate-entry-2` — Rate entry
- `canvas-pp-cli discussion-topics reorder` — Reorder pinned topics
- `canvas-pp-cli discussion-topics reorder-2` — Reorder pinned topics
- `canvas-pp-cli discussion-topics replies` — List entry replies
- `canvas-pp-cli discussion-topics replies-2` — List entry replies
- `canvas-pp-cli discussion-topics show` — Get a single topic
- `canvas-pp-cli discussion-topics show-2` — Get a single topic
- `canvas-pp-cli discussion-topics subscribe-topic` — Subscribe to a topic
- `canvas-pp-cli discussion-topics subscribe-topic-2` — Subscribe to a topic
- `canvas-pp-cli discussion-topics summary-feedback` — Summary Feedback
- `canvas-pp-cli discussion-topics summary-feedback-2` — Summary Feedback
- `canvas-pp-cli discussion-topics unsubscribe-topic` — Unsubscribe from a topic
- `canvas-pp-cli discussion-topics unsubscribe-topic-2` — Unsubscribe from a topic
- `canvas-pp-cli discussion-topics update` — Update a topic
- `canvas-pp-cli discussion-topics update-2` — Update a topic
- `canvas-pp-cli discussion-topics update-3` — Update an entry
- `canvas-pp-cli discussion-topics update-4` — Update an entry
- `canvas-pp-cli discussion-topics view` — Get the full topic
- `canvas-pp-cli discussion-topics view-2` — Get the full topic

**enrollment_terms** — API for viewing and managing enrollment terms. For all actions, the specified account must be a root account. To manage enrollment terms, the caller must have permission to manage the account. To view enrollment terms, the caller must have 

- `canvas-pp-cli enrollment-terms create` — Create enrollment term
- `canvas-pp-cli enrollment-terms destroy` — Delete enrollment term
- `canvas-pp-cli enrollment-terms index` — List enrollment terms
- `canvas-pp-cli enrollment-terms show` — Retrieve enrollment term
- `canvas-pp-cli enrollment-terms update` — Update enrollment term

**enrollments** — API for creating and viewing course enrollments

- `canvas-pp-cli enrollments accept` — Accept Course Invitation
- `canvas-pp-cli enrollments bulk-enrollment` — Enroll multiple users to one or more courses
- `canvas-pp-cli enrollments bulk-temporary-enrollment-status` — Bulk Temporary Enrollment Status
- `canvas-pp-cli enrollments create` — Enroll a user
- `canvas-pp-cli enrollments create-2` — Enroll a user
- `canvas-pp-cli enrollments destroy` — Conclude, deactivate, or delete an enrollment
- `canvas-pp-cli enrollments index` — List enrollments
- `canvas-pp-cli enrollments index-2` — List enrollments
- `canvas-pp-cli enrollments index-3` — List enrollments
- `canvas-pp-cli enrollments last-attended` — Add last attended date
- `canvas-pp-cli enrollments reactivate` — Re-activate an enrollment
- `canvas-pp-cli enrollments reject` — Reject Course Invitation
- `canvas-pp-cli enrollments show` — Enrollment by ID
- `canvas-pp-cli enrollments show-temporary-enrollment-status` — Show Temporary Enrollment recipient and provider status

**eportfolios** — {

- `canvas-pp-cli eportfolios delete` — Delete an ePortfolio
- `canvas-pp-cli eportfolios index` — Get all ePortfolios for a User
- `canvas-pp-cli eportfolios moderate` — Moderate an ePortfolio
- `canvas-pp-cli eportfolios moderate-all` — Moderate all ePortfolios for a User
- `canvas-pp-cli eportfolios pages` — Get ePortfolio Pages
- `canvas-pp-cli eportfolios restore` — Restore a deleted ePortfolio
- `canvas-pp-cli eportfolios show` — Get an ePortfolio

**epub_exports** — API for exporting courses as an ePub

- `canvas-pp-cli epub-exports create` — Create ePub Export
- `canvas-pp-cli epub-exports index` — List courses with their latest ePub export
- `canvas-pp-cli epub-exports show` — Show ePub export

**error_reports** — // A collection of information around a specific notification of a problem

- `canvas-pp-cli error-reports` — Create Error Report

**external_tools** — API for accessing and configuring external tools on accounts and courses. "External tools" are IMS LTI links: .

- `canvas-pp-cli external-tools add-top-nav-favorite` — Add tool to Top Navigation Favorites
- `canvas-pp-cli external-tools all-visible-nav-tools` — Get visible course navigation tools
- `canvas-pp-cli external-tools create` — Create an external tool
- `canvas-pp-cli external-tools create-2` — Create an external tool
- `canvas-pp-cli external-tools destroy` — Delete an external tool
- `canvas-pp-cli external-tools destroy-2` — Delete an external tool
- `canvas-pp-cli external-tools generate-sessionless-launch` — Get a sessionless launch url for an external tool.
- `canvas-pp-cli external-tools generate-sessionless-launch-2` — Get a sessionless launch url for an external tool.
- `canvas-pp-cli external-tools index` — List external tools
- `canvas-pp-cli external-tools index-2` — List external tools
- `canvas-pp-cli external-tools index-3` — List external tools
- `canvas-pp-cli external-tools mark-rce-favorite` — Mark tool as RCE Favorite
- `canvas-pp-cli external-tools remove-top-nav-favorite` — Remove tool from Top Navigation Favorites
- `canvas-pp-cli external-tools show` — Get a single external tool
- `canvas-pp-cli external-tools show-2` — Get a single external tool
- `canvas-pp-cli external-tools unmark-rce-favorite` — Unmark tool as RCE Favorite
- `canvas-pp-cli external-tools update` — Edit an external tool
- `canvas-pp-cli external-tools update-2` — Edit an external tool
- `canvas-pp-cli external-tools visible-course-nav-tools` — Get visible course navigation tools for a single course

**favorites** — {

- `canvas-pp-cli favorites add-favorite-course` — Add course to favorites
- `canvas-pp-cli favorites add-favorite-groups` — Add group to favorites
- `canvas-pp-cli favorites list-favorite-courses` — List favorite courses
- `canvas-pp-cli favorites list-favorite-groups` — List favorite groups
- `canvas-pp-cli favorites remove-favorite-course` — Remove course from favorites
- `canvas-pp-cli favorites remove-favorite-groups` — Remove group from favorites
- `canvas-pp-cli favorites reset-course-favorites` — Reset course favorites
- `canvas-pp-cli favorites reset-groups-favorites` — Reset group favorites

**feature_flags** — Manage optional features in Canvas.

- `canvas-pp-cli feature-flags delete` — Remove feature flag
- `canvas-pp-cli feature-flags delete-2` — Remove feature flag
- `canvas-pp-cli feature-flags delete-3` — Remove feature flag
- `canvas-pp-cli feature-flags enabled-features` — List enabled features
- `canvas-pp-cli feature-flags enabled-features-2` — List enabled features
- `canvas-pp-cli feature-flags enabled-features-3` — List enabled features
- `canvas-pp-cli feature-flags environment` — List environment features
- `canvas-pp-cli feature-flags index` — List features
- `canvas-pp-cli feature-flags index-2` — List features
- `canvas-pp-cli feature-flags index-3` — List features
- `canvas-pp-cli feature-flags show` — Get feature flag
- `canvas-pp-cli feature-flags show-2` — Get feature flag
- `canvas-pp-cli feature-flags show-3` — Get feature flag
- `canvas-pp-cli feature-flags update` — Set feature flag
- `canvas-pp-cli feature-flags update-2` — Set feature flag
- `canvas-pp-cli feature-flags update-3` — Set feature flag

**files** — An API for managing files and folders See the File Upload Documentation for details on the file upload workflow.

- `canvas-pp-cli files api-destroy` — Delete folder
- `canvas-pp-cli files api-index` — List files
- `canvas-pp-cli files api-index-2` — List files
- `canvas-pp-cli files api-index-3` — List files
- `canvas-pp-cli files api-index-4` — List files
- `canvas-pp-cli files api-index-5` — List folders
- `canvas-pp-cli files api-quota` — Get quota information
- `canvas-pp-cli files api-quota-2` — Get quota information
- `canvas-pp-cli files api-quota-3` — Get quota information
- `canvas-pp-cli files api-show` — Get file
- `canvas-pp-cli files api-show-2` — Get file
- `canvas-pp-cli files api-show-3` — Get file
- `canvas-pp-cli files api-show-4` — Get file
- `canvas-pp-cli files api-show-5` — Get file
- `canvas-pp-cli files api-update` — Update file
- `canvas-pp-cli files copy-file` — Copy a file
- `canvas-pp-cli files copy-folder` — Copy a folder
- `canvas-pp-cli files create` — Create folder
- `canvas-pp-cli files create-2` — Create folder
- `canvas-pp-cli files create-3` — Create folder
- `canvas-pp-cli files create-4` — Create folder
- `canvas-pp-cli files create-5` — Create folder
- `canvas-pp-cli files create-file` — Upload a file
- `canvas-pp-cli files destroy` — Delete file
- `canvas-pp-cli files file-ref` — Translate file reference
- `canvas-pp-cli files icon-metadata` — Get icon metadata
- `canvas-pp-cli files licenses` — List licenses
- `canvas-pp-cli files licenses-2` — List licenses
- `canvas-pp-cli files licenses-3` — List licenses
- `canvas-pp-cli files list-all-folders` — List all folders
- `canvas-pp-cli files list-all-folders-2` — List all folders
- `canvas-pp-cli files list-all-folders-3` — List all folders
- `canvas-pp-cli files media-folder` — Get uploaded media folder for user
- `canvas-pp-cli files media-folder-2` — Get uploaded media folder for user
- `canvas-pp-cli files public-url` — Get public inline preview url
- `canvas-pp-cli files remove-usage-rights` — Remove usage rights
- `canvas-pp-cli files remove-usage-rights-2` — Remove usage rights
- `canvas-pp-cli files remove-usage-rights-3` — Remove usage rights
- `canvas-pp-cli files reset-verifier` — Reset link verifier
- `canvas-pp-cli files resolve-path` — Resolve path
- `canvas-pp-cli files resolve-path-2` — Resolve path
- `canvas-pp-cli files resolve-path-3` — Resolve path
- `canvas-pp-cli files resolve-path-4` — Resolve path
- `canvas-pp-cli files resolve-path-5` — Resolve path
- `canvas-pp-cli files resolve-path-6` — Resolve path
- `canvas-pp-cli files set-usage-rights` — Set usage rights
- `canvas-pp-cli files set-usage-rights-2` — Set usage rights
- `canvas-pp-cli files set-usage-rights-3` — Set usage rights
- `canvas-pp-cli files show` — Get folder
- `canvas-pp-cli files show-2` — Get folder
- `canvas-pp-cli files show-3` — Get folder
- `canvas-pp-cli files show-4` — Get folder
- `canvas-pp-cli files update` — Update folder

**grade_change_log** — Query audit log of grade change events.

- `canvas-pp-cli grade-change-log for-assignment` — Query by assignment
- `canvas-pp-cli grade-change-log for-course` — Query by course
- `canvas-pp-cli grade-change-log for-grader` — Query by grader
- `canvas-pp-cli grade-change-log for-student` — Query by student
- `canvas-pp-cli grade-change-log query` — Advanced query

**gradebook_history** — API for accessing the versioned history of student submissions along with their grade changes, organized by the date of the submission.

- `canvas-pp-cli gradebook-history day-details` — Details for a given date in gradebook history for this course
- `canvas-pp-cli gradebook-history days` — Days in gradebook history for this course
- `canvas-pp-cli gradebook-history feed` — List uncollated submission versions
- `canvas-pp-cli gradebook-history submissions` — Lists submissions

**grading_period_sets** — Manage grading period sets

- `canvas-pp-cli grading-period-sets create` — Create a grading period set
- `canvas-pp-cli grading-period-sets destroy` — Delete a grading period set
- `canvas-pp-cli grading-period-sets index` — List grading period sets
- `canvas-pp-cli grading-period-sets update` — Update a grading period set

**grading_periods** — Manage grading periods

- `canvas-pp-cli grading-periods batch-update` — Batch update grading periods
- `canvas-pp-cli grading-periods batch-update-2` — Batch update grading periods
- `canvas-pp-cli grading-periods destroy` — Delete a grading period
- `canvas-pp-cli grading-periods destroy-2` — Delete a grading period
- `canvas-pp-cli grading-periods index` — List grading periods
- `canvas-pp-cli grading-periods index-2` — List grading periods
- `canvas-pp-cli grading-periods show` — Get a single grading period
- `canvas-pp-cli grading-periods update` — Update a single grading period

**grading_standards** — {

- `canvas-pp-cli grading-standards context-index` — List the grading standards available in a context.
- `canvas-pp-cli grading-standards context-index-2` — List the grading standards available in a context.
- `canvas-pp-cli grading-standards context-show` — Get a single grading standard in a context.
- `canvas-pp-cli grading-standards context-show-2` — Get a single grading standard in a context.
- `canvas-pp-cli grading-standards create` — Create a new grading standard
- `canvas-pp-cli grading-standards create-2` — Create a new grading standard
- `canvas-pp-cli grading-standards destroy` — Delete a grading standard
- `canvas-pp-cli grading-standards destroy-2` — Delete a grading standard
- `canvas-pp-cli grading-standards update` — Update a grading standard
- `canvas-pp-cli grading-standards update-2` — Update a grading standard

**group_categories** — Group Categories allow grouping of groups together in canvas. There are a few different built-in group categories used, or custom ones can be created. The built in group categories are: "communities", "student_organized", and "imported".

- `canvas-pp-cli group-categories assign-unassigned-members` — Assign unassigned members
- `canvas-pp-cli group-categories bulk-manage-differentiation-tag` — Bulk manage differentiation tags
- `canvas-pp-cli group-categories create` — Create a Group Category
- `canvas-pp-cli group-categories create-2` — Create a Group Category
- `canvas-pp-cli group-categories destroy` — Delete a Group Category
- `canvas-pp-cli group-categories export` — export groups in and users in category
- `canvas-pp-cli group-categories export-tags` — export tags and users in course
- `canvas-pp-cli group-categories groups` — List groups in group category
- `canvas-pp-cli group-categories import` — Import category groups
- `canvas-pp-cli group-categories import-tags` — Import differentiation tags
- `canvas-pp-cli group-categories index` — List group categories for a context
- `canvas-pp-cli group-categories index-2` — List group categories for a context
- `canvas-pp-cli group-categories show` — Get a single group category
- `canvas-pp-cli group-categories update` — Update a Group Category
- `canvas-pp-cli group-categories users` — List users in group category

**groups** — Groups serve as the data for a few different ideas in Canvas. The first is that they can be a community in the canvas network. The second is that they can be organized by students in a course, for study or communication (but not grading). T

- `canvas-pp-cli groups activity-stream` — Group activity stream
- `canvas-pp-cli groups activity-stream-summary` — Group activity stream summary
- `canvas-pp-cli groups bulk-user-tags` — Bulk fetch user tags for multiple users in a course
- `canvas-pp-cli groups context-index` — List the groups available in a context.
- `canvas-pp-cli groups context-index-2` — List the groups available in a context.
- `canvas-pp-cli groups create` — Create a group
- `canvas-pp-cli groups create-2` — Create a group
- `canvas-pp-cli groups create-3` — Create a membership
- `canvas-pp-cli groups create-file` — Upload a file
- `canvas-pp-cli groups delete-users` — ***
- `canvas-pp-cli groups destroy` — Delete a group
- `canvas-pp-cli groups destroy-2` — Leave a group
- `canvas-pp-cli groups destroy-3` — Leave a group
- `canvas-pp-cli groups index` — List your groups
- `canvas-pp-cli groups index-2` — List group memberships
- `canvas-pp-cli groups invite` — Invite others to a group
- `canvas-pp-cli groups permissions` — Permissions
- `canvas-pp-cli groups preview-html` — Preview processed html
- `canvas-pp-cli groups show` — Get a single group
- `canvas-pp-cli groups show-2` — Get a single group membership
- `canvas-pp-cli groups show-3` — Get a single group membership
- `canvas-pp-cli groups update` — Edit a group
- `canvas-pp-cli groups update-2` — Update a membership
- `canvas-pp-cli groups update-3` — Update a membership
- `canvas-pp-cli groups users` — List group's users

**history_resource** — // Information about a recently visited item or page in Canvas

- `canvas-pp-cli history-resource <user_id>` — List recent history for a user

**instaccess_tokens** — Short term JWT tokens that can be used to authenticate with Canvas and other Instructure services. InstAccess tokens expire after one hour. Canvas hands out encrypted tokens that need to be decrypted by the API Gateway before they can be ac

- `canvas-pp-cli instaccess-tokens` — Create InstAccess token

**jwts** — Short term tokens useful for talking to other services in the Canvas Ecosystem. Note: JWTs have no value or use directly against the Canvas API, and expire after one hour

- `canvas-pp-cli jwts create` — Create JWT
- `canvas-pp-cli jwts refresh` — Refresh JWT

**late_policy** — Manage a course's late policy.

- `canvas-pp-cli late-policy create` — Create a late policy
- `canvas-pp-cli late-policy show` — Get a late policy
- `canvas-pp-cli late-policy update` — Patch a late policy

**learning_object_dates** — API for accessing date-related attributes on assignments, quizzes, modules, discussions, pages, and files. Note that support for files is not yet available.

- `canvas-pp-cli learning-object-dates show` — Get a learning object's date information
- `canvas-pp-cli learning-object-dates show-2` — Get a learning object's date information
- `canvas-pp-cli learning-object-dates show-3` — Get a learning object's date information
- `canvas-pp-cli learning-object-dates show-4` — Get a learning object's date information
- `canvas-pp-cli learning-object-dates show-5` — Get a learning object's date information
- `canvas-pp-cli learning-object-dates show-6` — Get a learning object's date information
- `canvas-pp-cli learning-object-dates update` — Update a learning object's date information
- `canvas-pp-cli learning-object-dates update-2` — Update a learning object's date information
- `canvas-pp-cli learning-object-dates update-3` — Update a learning object's date information
- `canvas-pp-cli learning-object-dates update-4` — Update a learning object's date information
- `canvas-pp-cli learning-object-dates update-5` — Update a learning object's date information

**liveassessments** — Manage live assessment results

- `canvas-pp-cli liveassessments get-live-assessments` — Returns a paginated list of live assessments.
- `canvas-pp-cli liveassessments get-results` — Returns a paginated list of live assessment results
- `canvas-pp-cli liveassessments post-live-assessments` — Creates or finds an existing live assessment with the given key and aligns it with the linked outcome
- `canvas-pp-cli liveassessments post-results` — Creates live assessment results and adds them to a live assessment

**logins** — API for creating and viewing user logins under an account

- `canvas-pp-cli logins create` — Create a user login
- `canvas-pp-cli logins destroy` — Delete a user login
- `canvas-pp-cli logins forgot-password` — Kickoff password recovery flow
- `canvas-pp-cli logins index` — List user logins
- `canvas-pp-cli logins index-2` — List user logins
- `canvas-pp-cli logins update` — Edit a user login

**lti_contextcontrols** — Configure the availability of an LTI Registration in a specific context. Used by the Canvas Apps page UI.

- `canvas-pp-cli lti-contextcontrols delete-controls` — Deletes a context control. Returns the control that is now deleted.
- `canvas-pp-cli lti-contextcontrols get-controls` — List all LTI ContextControls for the given LTI Registration. These controls are partitioned by LTI Deployment, and have
- `canvas-pp-cli lti-contextcontrols get-controls-2` — Display details of the specified LTI ContextControl for the specified LTI registration in this context.
- `canvas-pp-cli lti-contextcontrols post-bulk` — Create up to 100 new LTI ContextControls for the specified LTI registration in this context. Control parameters are sent
- `canvas-pp-cli lti-contextcontrols post-controls` — Create a new LTI ContextControl for the specified LTI registration in this context.
- `canvas-pp-cli lti-contextcontrols put-controls` — Changes the availability of a context control. This endpoint can only be used to change the availability of a context co

**lti_launch_definitions** — // A bare-bones representation of an LTI tool used by Canvas to launch the tool

- `canvas-pp-cli lti-launch-definitions get-launch-definitions` — List all tools available in this context for the given placements, in the form of Launch Definitions. Used primarily by
- `canvas-pp-cli lti-launch-definitions get-launch-definitions-2` — List all tools available in this context for the given placements, in the form of Launch Definitions. Used primarily by

**lti_registrations** — {% hint style="warning" %}

- `canvas-pp-cli lti-registrations delete-bind` — Deletes the account binding for this registration, effectively removing it from the account.
- `canvas-pp-cli lti-registrations delete-lti-registrations` — Remove the specified LTI registration
- `canvas-pp-cli lti-registrations get-by-utid` — Returns an LTI registration by looking up its unified_tool_id. Searches both manual configurations and IMS registrations
- `canvas-pp-cli lti-registrations get-context-search` — This is a utility endpoint used by the Canvas Apps UI and may not serve general use cases.
- `canvas-pp-cli lti-registrations get-history` — Returns the history entries for the specified LTI registration. This endpoint provides comprehensive change tracking for
- `canvas-pp-cli lti-registrations get-install-status` — Returns the local installation status for a Site Admin LTI registration. If the developer key's registration is in Site
- `canvas-pp-cli lti-registrations get-latest-update-request` — Retrieves the most recent update request for a registration, regardless of its status. Returns 404 if there are no updat
- `canvas-pp-cli lti-registrations get-lti-registration-by-client-id` — Returns details about the specified LTI registration, including the configuration and account binding.
- `canvas-pp-cli lti-registrations get-lti-registrations` — Returns all LTI registrations in the specified account. Includes registrations created in this account, those set to 'al
- `canvas-pp-cli lti-registrations get-lti-registrations-2` — Return details about the specified LTI registration, including the configuration and account binding.
- `canvas-pp-cli lti-registrations get-overlay-history` — Returns the overlay history items for the specified LTI registration.
- `canvas-pp-cli lti-registrations get-update-requests` — Retrieves details about a specific registration update request.
- `canvas-pp-cli lti-registrations post-bind` — Enable or disable the specified LTI registration for the specified root account. To enable an inherited registration (eg
- `canvas-pp-cli lti-registrations post-install-from-template` — This endpoint installs a local copy of a 'template' LTI registration from Site Admin into the specified account. The loc
- `canvas-pp-cli lti-registrations post-lti-registrations` — Create a new LTI Registration, as well as an associated Tool Configuration, Developer Key, and Registration Account bind
- `canvas-pp-cli lti-registrations put-apply` — Applies a registration update request to an existing registration, replacing the existing configuration and overlay with
- `canvas-pp-cli lti-registrations put-lti-registrations` — Update the specified LTI registration with the provided parameters. Note that updating the base tool configuration of a
- `canvas-pp-cli lti-registrations put-reset` — Reset the specified LTI registration to its default settings in this context. This removes all customizations that were

**lti_resource_links** — API that exposes LTI Resource Links for viewing and editing. LTI Resource Links are artifacts created by the LTI 1.3 Deep Linking process, where a user selects a content item that is returned to Canvas for future launches.

- `canvas-pp-cli lti-resource-links delete-lti-resource-links` — Delete the specified resource link. The ID can be in the standard Canvas format ('1'), or in these special formats:
- `canvas-pp-cli lti-resource-links get-lti-resource-links` — Returns all Resource Links in the specified course. This includes links that are associated with Assignments, Module Ite
- `canvas-pp-cli lti-resource-links get-lti-resource-links-2` — Return details about the specified resource link. The ID can be in the standard Canvas format ('1'), or in these special
- `canvas-pp-cli lti-resource-links post-bulk` — Create up to 100 new LTI Resource Links in the specified course with the provided parameters.
- `canvas-pp-cli lti-resource-links post-lti-resource-links` — Create a new LTI Resource Link in the specified course with the provided parameters.
- `canvas-pp-cli lti-resource-links put-lti-resource-links` — Update the specified resource link with the provided parameters.

**media_objects** — Closed captions added to a video MediaObject

- `canvas-pp-cli media-objects index` — List media tracks for a Media Object or Attachment
- `canvas-pp-cli media-objects index-2` — List media tracks for a Media Object or Attachment
- `canvas-pp-cli media-objects index-3` — List Media Objects
- `canvas-pp-cli media-objects index-4` — List Media Objects
- `canvas-pp-cli media-objects index-5` — List Media Objects
- `canvas-pp-cli media-objects index-6` — List Media Objects
- `canvas-pp-cli media-objects index-7` — List Media Objects
- `canvas-pp-cli media-objects index-8` — List Media Objects
- `canvas-pp-cli media-objects update` — Update Media Tracks
- `canvas-pp-cli media-objects update-2` — Update Media Tracks
- `canvas-pp-cli media-objects update-media-object` — Update Media Object
- `canvas-pp-cli media-objects update-media-object-2` — Update Media Object

**moderated_grading** — API for viewing and adding students to the list of people in moderation for an assignment

- `canvas-pp-cli moderated-grading bulk-select` — Bulk select provisional grades
- `canvas-pp-cli moderated-grading create` — Select students for moderation
- `canvas-pp-cli moderated-grading index` — List students selected for moderation
- `canvas-pp-cli moderated-grading publish` — Publish provisional grades for an assignment
- `canvas-pp-cli moderated-grading select` — Select provisional grade
- `canvas-pp-cli moderated-grading status` — Show provisional grade status for a student
- `canvas-pp-cli moderated-grading status-2` — Show provisional grade status for a student

**modules** — Modules are collections of learning materials useful for organizing courses and optionally providing a linear flow through them. Module items can be accessed linearly or sequentially depending on module configuration. Items can be unlocked 

- `canvas-pp-cli modules bulk-update` — Update a module's overrides
- `canvas-pp-cli modules create` — Create a module
- `canvas-pp-cli modules create-2` — Create a module item
- `canvas-pp-cli modules destroy` — Delete module
- `canvas-pp-cli modules destroy-2` — Delete module item
- `canvas-pp-cli modules index` — List modules
- `canvas-pp-cli modules index-2` — List module items
- `canvas-pp-cli modules index-3` — List a module's overrides
- `canvas-pp-cli modules item-sequence` — Get module item sequence
- `canvas-pp-cli modules mark-as-done` — Mark module item as done/not done
- `canvas-pp-cli modules mark-item-read` — Mark module item read
- `canvas-pp-cli modules relock` — Re-lock module progressions
- `canvas-pp-cli modules select-mastery-path` — Select a mastery path
- `canvas-pp-cli modules show` — Show module
- `canvas-pp-cli modules show-2` — Show module item
- `canvas-pp-cli modules update` — Update a module
- `canvas-pp-cli modules update-2` — Update a module item

**notification_preferences** — API for managing notification preferences

- `canvas-pp-cli notification-preferences category-index` — List of preference categories
- `canvas-pp-cli notification-preferences index` — List preferences
- `canvas-pp-cli notification-preferences index-2` — List preferences
- `canvas-pp-cli notification-preferences show` — Get a preference
- `canvas-pp-cli notification-preferences show-2` — Get a preference
- `canvas-pp-cli notification-preferences update` — Update a preference
- `canvas-pp-cli notification-preferences update-2` — Update a preference
- `canvas-pp-cli notification-preferences update-all` — Update multiple preferences
- `canvas-pp-cli notification-preferences update-all-2` — Update multiple preferences
- `canvas-pp-cli notification-preferences update-preferences-by-category` — Update preferences by category

**outcome_groups** — API for accessing learning outcome group information.

- `canvas-pp-cli outcome-groups create` — Create a subgroup
- `canvas-pp-cli outcome-groups create-2` — Create a subgroup
- `canvas-pp-cli outcome-groups create-3` — Create a subgroup
- `canvas-pp-cli outcome-groups destroy` — Delete an outcome group
- `canvas-pp-cli outcome-groups destroy-2` — Delete an outcome group
- `canvas-pp-cli outcome-groups destroy-3` — Delete an outcome group
- `canvas-pp-cli outcome-groups import` — Import an outcome group
- `canvas-pp-cli outcome-groups import-2` — Import an outcome group
- `canvas-pp-cli outcome-groups import-3` — Import an outcome group
- `canvas-pp-cli outcome-groups index` — Get all outcome groups for context
- `canvas-pp-cli outcome-groups index-2` — Get all outcome groups for context
- `canvas-pp-cli outcome-groups link` — Create/link an outcome
- `canvas-pp-cli outcome-groups link-2` — Create/link an outcome
- `canvas-pp-cli outcome-groups link-3` — Create/link an outcome
- `canvas-pp-cli outcome-groups link-4` — Create/link an outcome
- `canvas-pp-cli outcome-groups link-5` — Create/link an outcome
- `canvas-pp-cli outcome-groups link-6` — Create/link an outcome
- `canvas-pp-cli outcome-groups link-index` — Get all outcome links for context
- `canvas-pp-cli outcome-groups link-index-2` — Get all outcome links for context
- `canvas-pp-cli outcome-groups outcomes` — List linked outcomes
- `canvas-pp-cli outcome-groups outcomes-2` — List linked outcomes
- `canvas-pp-cli outcome-groups outcomes-3` — List linked outcomes
- `canvas-pp-cli outcome-groups redirect` — Redirect to root outcome group for context
- `canvas-pp-cli outcome-groups redirect-2` — Redirect to root outcome group for context
- `canvas-pp-cli outcome-groups redirect-3` — Redirect to root outcome group for context
- `canvas-pp-cli outcome-groups show` — Show an outcome group
- `canvas-pp-cli outcome-groups show-2` — Show an outcome group
- `canvas-pp-cli outcome-groups show-3` — Show an outcome group
- `canvas-pp-cli outcome-groups subgroups` — List subgroups
- `canvas-pp-cli outcome-groups subgroups-2` — List subgroups
- `canvas-pp-cli outcome-groups subgroups-3` — List subgroups
- `canvas-pp-cli outcome-groups unlink` — Unlink an outcome
- `canvas-pp-cli outcome-groups unlink-2` — Unlink an outcome
- `canvas-pp-cli outcome-groups unlink-3` — Unlink an outcome
- `canvas-pp-cli outcome-groups update` — Update an outcome group
- `canvas-pp-cli outcome-groups update-2` — Update an outcome group
- `canvas-pp-cli outcome-groups update-3` — Update an outcome group

**outcome_imports** — API for importing outcome data

- `canvas-pp-cli outcome-imports create` — Import Outcomes
- `canvas-pp-cli outcome-imports create-2` — Import Outcomes
- `canvas-pp-cli outcome-imports created-group-ids` — Get IDs of outcome groups created after successful import
- `canvas-pp-cli outcome-imports created-group-ids-2` — Get IDs of outcome groups created after successful import
- `canvas-pp-cli outcome-imports show` — Get Outcome import status
- `canvas-pp-cli outcome-imports show-2` — Get Outcome import status

**outcome_results** — API for accessing learning outcome results

- `canvas-pp-cli outcome-results contributing-scores` — Get contributing scores
- `canvas-pp-cli outcome-results enqueue-outcome-rollup-calculation` — Enqueue a delayed Outcome Rollup Calculation Job
- `canvas-pp-cli outcome-results index` — Get outcome results
- `canvas-pp-cli outcome-results mastery-distribution` — Get mastery distribution
- `canvas-pp-cli outcome-results outcome-order` — Set outcome ordering for LMGB
- `canvas-pp-cli outcome-results rollups` — Get outcome result rollups

**outcomes** — API for accessing learning outcome information.

- `canvas-pp-cli outcomes outcome-alignments` — Get outcome alignments for a student or assignment
- `canvas-pp-cli outcomes show` — Show an outcome
- `canvas-pp-cli outcomes update` — Update an outcome

**pages** — Pages are rich content associated with Courses and Groups in Canvas. The Pages API allows you to create, retrieve, update, and delete pages.

- `canvas-pp-cli pages create` — Create page
- `canvas-pp-cli pages create-2` — Create page
- `canvas-pp-cli pages destroy` — Delete page
- `canvas-pp-cli pages destroy-2` — Delete page
- `canvas-pp-cli pages duplicate` — Duplicate page
- `canvas-pp-cli pages index` — List pages
- `canvas-pp-cli pages index-2` — List pages
- `canvas-pp-cli pages revert` — Revert to revision
- `canvas-pp-cli pages revert-2` — Revert to revision
- `canvas-pp-cli pages revisions` — List revisions
- `canvas-pp-cli pages revisions-2` — List revisions
- `canvas-pp-cli pages show` — Show page
- `canvas-pp-cli pages show-2` — Show page
- `canvas-pp-cli pages show-front-page` — Show front page
- `canvas-pp-cli pages show-front-page-2` — Show front page
- `canvas-pp-cli pages show-revision` — Show revision
- `canvas-pp-cli pages show-revision-2` — Show revision
- `canvas-pp-cli pages show-revision-3` — Show revision
- `canvas-pp-cli pages show-revision-4` — Show revision
- `canvas-pp-cli pages update` — Update/create page
- `canvas-pp-cli pages update-2` — Update/create page
- `canvas-pp-cli pages update-front-page` — Update/create front page
- `canvas-pp-cli pages update-front-page-2` — Update/create front page

**peer_reviews** — {

- `canvas-pp-cli peer-reviews allocate` — Allocate Peer Review
- `canvas-pp-cli peer-reviews create` — Create Peer Review
- `canvas-pp-cli peer-reviews create-2` — Create Peer Review
- `canvas-pp-cli peer-reviews destroy` — Delete Peer Review
- `canvas-pp-cli peer-reviews destroy-2` — Delete Peer Review
- `canvas-pp-cli peer-reviews index` — Get all Peer Reviews
- `canvas-pp-cli peer-reviews index-2` — Get all Peer Reviews
- `canvas-pp-cli peer-reviews index-3` — Get all Peer Reviews
- `canvas-pp-cli peer-reviews index-4` — Get all Peer Reviews

**planner** — API for listing learning objects to display on the student planner and calendar

- `canvas-pp-cli planner create` — Create a planner note
- `canvas-pp-cli planner create-2` — Create a planner override
- `canvas-pp-cli planner destroy` — Delete a planner note
- `canvas-pp-cli planner destroy-2` — Delete a planner override
- `canvas-pp-cli planner index` — List planner items
- `canvas-pp-cli planner index-2` — List planner items
- `canvas-pp-cli planner index-3` — List planner notes
- `canvas-pp-cli planner index-4` — List planner overrides
- `canvas-pp-cli planner show` — Show a planner note
- `canvas-pp-cli planner show-2` — Show a planner override
- `canvas-pp-cli planner update` — Update a planner note
- `canvas-pp-cli planner update-2` — Update a planner override

**poll_sessions** — Manage poll sessions

- `canvas-pp-cli poll-sessions delete-poll-sessions` — 204 No Content response code is returned if the deletion was successful.
- `canvas-pp-cli poll-sessions get-close` — /api/v1/polls/{poll_id}/poll_sessions/{id}/close
- `canvas-pp-cli poll-sessions get-closed` — A paginated list of all closed poll sessions available to the current user.
- `canvas-pp-cli poll-sessions get-open` — /api/v1/polls/{poll_id}/poll_sessions/{id}/open
- `canvas-pp-cli poll-sessions get-opened` — A paginated list of all opened poll sessions available to the current user.
- `canvas-pp-cli poll-sessions get-poll-sessions` — Returns the paginated list of PollSessions in this poll.
- `canvas-pp-cli poll-sessions get-poll-sessions-2` — Returns the poll session with the given id
- `canvas-pp-cli poll-sessions post-poll-sessions` — Create a new poll session for this poll
- `canvas-pp-cli poll-sessions put-poll-sessions` — Update an existing poll session for this poll

**pollchoices** — Manage choices for polls

- `canvas-pp-cli pollchoices delete-poll-choices` — 204 No Content response code is returned if the deletion was successful.
- `canvas-pp-cli pollchoices get-poll-choices` — Returns the paginated list of PollChoices in this poll.
- `canvas-pp-cli pollchoices get-poll-choices-2` — Returns the poll choice with the given id
- `canvas-pp-cli pollchoices post-poll-choices` — Create a new poll choice for this poll
- `canvas-pp-cli pollchoices put-poll-choices` — Update an existing poll choice for this poll

**polls** — Manage polls

- `canvas-pp-cli polls delete-polls` — 204 No Content response code is returned if the deletion was successful.
- `canvas-pp-cli polls get-polls` — Returns the paginated list of polls for the current user.
- `canvas-pp-cli polls get-polls-2` — Returns the poll with the given id
- `canvas-pp-cli polls post-polls` — Create a new poll for the current user
- `canvas-pp-cli polls put-polls` — Update an existing poll belonging to the current user

**pollsubmissions** — Manage submissions for polls

- `canvas-pp-cli pollsubmissions get-poll-submissions` — Returns the poll submission with the given id
- `canvas-pp-cli pollsubmissions post-poll-submissions` — Create a new poll submission for this poll session

**proficiency_ratings** — API for customizing proficiency ratings

- `canvas-pp-cli proficiency-ratings create` — Create/update proficiency ratings
- `canvas-pp-cli proficiency-ratings create-2` — Create/update proficiency ratings
- `canvas-pp-cli proficiency-ratings show` — Get proficiency ratings
- `canvas-pp-cli proficiency-ratings show-2` — Get proficiency ratings

**progress** — API for querying the progress of asynchronous API operations.

- `canvas-pp-cli progress cancel` — Cancel progress
- `canvas-pp-cli progress show` — Query progress

**quiz_assignment_overrides** — // Set of assignment-overridden dates for a quiz.

- `canvas-pp-cli quiz-assignment-overrides get-assignment-overrides` — Retrieve the actual due-at, unlock-at, and available-at dates for quizzes based on the assignment overrides active for t
- `canvas-pp-cli quiz-assignment-overrides get-assignment-overrides-2` — Retrieve the actual due-at, unlock-at, and available-at dates for quizzes based on the assignment overrides active for t

**quiz_extensions** — API for setting extensions on student quiz submissions

- `canvas-pp-cli quiz-extensions <course_id> <quiz_id>` — Responses

**quiz_ip_filters** — API for accessing quiz IP filters

- `canvas-pp-cli quiz-ip-filters <course_id> <quiz_id>` — Get a list of available IP filters for this Quiz.

**quiz_question_groups** — API for accessing information on quiz question groups

- `canvas-pp-cli quiz-question-groups delete-groups` — Delete a question group
- `canvas-pp-cli quiz-question-groups get-groups` — Returns a list of question groups in a quiz.
- `canvas-pp-cli quiz-question-groups get-groups-2` — Returns details of the quiz group with the given id.
- `canvas-pp-cli quiz-question-groups post-groups` — Create a new question group for this quiz
- `canvas-pp-cli quiz-question-groups post-reorder` — Change the order of the quiz questions within the group
- `canvas-pp-cli quiz-question-groups put-groups` — Update a question group

**quiz_questions** — {

- `canvas-pp-cli quiz-questions delete-questions` — 204 No Content response code is returned if the deletion was successful.
- `canvas-pp-cli quiz-questions get-questions` — Returns the paginated list of QuizQuestions in this quiz.
- `canvas-pp-cli quiz-questions get-questions-2` — Returns the quiz question with the given id
- `canvas-pp-cli quiz-questions post-questions` — Create a new quiz question for this quiz
- `canvas-pp-cli quiz-questions put-questions` — Updates an existing quiz question for this quiz

**quiz_reports** — API for accessing and generating statistical reports for a quiz

- `canvas-pp-cli quiz-reports delete-reports` — This API allows you to cancel a previous request you issued for a report to be generated. Or in the case of an already g
- `canvas-pp-cli quiz-reports get-reports` — Returns a list of all available reports.
- `canvas-pp-cli quiz-reports get-reports-2` — Returns the data for a single quiz report.
- `canvas-pp-cli quiz-reports post-reports` — Create and return a new report for this quiz. If a previously generated report matches the arguments and is still curren

**quiz_statistics** — API for accessing quiz submission statistics. The statistics provided by this interface are an aggregate of what is known as Student and Item Analysis for a quiz.

- `canvas-pp-cli quiz-statistics <course_id> <quiz_id>` — This endpoint provides statistics for all quiz versions, or for a specific quiz version, in which case the output is gua

**quiz_submission_events** — // An event passed from the Quiz Submission take page

- `canvas-pp-cli quiz-submission-events get-events` — Retrieve the set of events captured during a specific submission attempt.
- `canvas-pp-cli quiz-submission-events post-events` — Store a set of events which were captured during a quiz taking session.

**quiz_submission_files** — [Quizzes::QuizSubmissionFilesController#create](https://github.com/instructure/canvas-lms/blob/master/app/controllers/quizzes/quiz_submission_files_controller.rb)

- `canvas-pp-cli quiz-submission-files <course_id> <quiz_id>` — Associate a new quiz submission file

**quiz_submission_questions** — API for answering and flagging questions in a quiz-taking session.

- `canvas-pp-cli quiz-submission-questions get-formatted-answer` — Matches the intended behavior of the UI when a numerical answer is entered and returns the resulting formatted number
- `canvas-pp-cli quiz-submission-questions get-questions` — Get a list of all the question records for this quiz submission.
- `canvas-pp-cli quiz-submission-questions post-questions` — Provide or update an answer to one or more QuizQuestions.
- `canvas-pp-cli quiz-submission-questions put-flag` — Set a flag on a quiz question to indicate that you want to return to it later.
- `canvas-pp-cli quiz-submission-questions put-unflag` — Remove the flag that you previously set on a quiz question after you've returned to it.

**quiz_submission_user_list** — List of users who have or haven't submitted for a quiz.

- `canvas-pp-cli quiz-submission-user-list <course_id> <id>` — { 'body': { 'type': 'string', 'description': 'message body of the conversation to be created', 'example': 'Please take t

**quiz_submissions** — API for accessing quiz submissions

- `canvas-pp-cli quiz-submissions get-submission` — Get the submission for this quiz for the current user.
- `canvas-pp-cli quiz-submissions get-submissions` — Get a list of all submissions for this quiz. Users who can view or manage grades for a course will have submissions from
- `canvas-pp-cli quiz-submissions get-submissions-2` — Get a single quiz submission.
- `canvas-pp-cli quiz-submissions get-time` — Get the current timing data for the quiz attempt, both the end_at timestamp and the time_left parameter.
- `canvas-pp-cli quiz-submissions post-complete` — Complete the quiz submission by marking it as complete and grading it. When the quiz submission has been marked as compl
- `canvas-pp-cli quiz-submissions post-submissions` — Start taking a Quiz by creating a QuizSubmission which you can use to answer questions and submit your answers.
- `canvas-pp-cli quiz-submissions put-submissions` — Update the amount of points a student has scored for questions they've answered, provide comments for the student about

**quizzes** — {

- `canvas-pp-cli quizzes delete-quizzes` — Deletes a quiz and returns the deleted quiz object.
- `canvas-pp-cli quizzes get-quizzes` — Returns the paginated list of Quizzes in this course.
- `canvas-pp-cli quizzes get-quizzes-2` — Returns the quiz with the given id.
- `canvas-pp-cli quizzes post-quizzes` — Create a new quiz for this course.
- `canvas-pp-cli quizzes post-reorder` — Change order of the quiz questions or groups within the quiz
- `canvas-pp-cli quizzes post-validate-access-code` — Accepts an access code and returns a boolean indicating whether that access code is correct
- `canvas-pp-cli quizzes put-quizzes` — Modify an existing quiz. See the documentation for quiz creation.

**roles** — API for managing account- and course-level roles, and their associated permissions.

- `canvas-pp-cli roles activate-role` — Activate a role
- `canvas-pp-cli roles add-role` — Create a new role
- `canvas-pp-cli roles api-index` — List roles
- `canvas-pp-cli roles groups` — Retrieve permission groups
- `canvas-pp-cli roles help` — Get help text for permissions
- `canvas-pp-cli roles manageable-permissions` — List assignable permissions
- `canvas-pp-cli roles remove-role` — Deactivate a role
- `canvas-pp-cli roles show` — Get a single role
- `canvas-pp-cli roles update` — Update a role

**rubrics** — API for accessing rubric information.

- `canvas-pp-cli rubrics create` — Create a single rubric
- `canvas-pp-cli rubrics create-2` — Create a single rubric assessment
- `canvas-pp-cli rubrics create-3` — Create a RubricAssociation
- `canvas-pp-cli rubrics destroy` — Delete a single
- `canvas-pp-cli rubrics destroy-2` — Delete a single rubric assessment
- `canvas-pp-cli rubrics destroy-3` — Delete a RubricAssociation
- `canvas-pp-cli rubrics index` — List rubrics
- `canvas-pp-cli rubrics index-2` — List rubrics
- `canvas-pp-cli rubrics show` — Get a single rubric
- `canvas-pp-cli rubrics show-2` — Get a single rubric
- `canvas-pp-cli rubrics update` — Update a single rubric
- `canvas-pp-cli rubrics update-2` — Update a single rubric assessment
- `canvas-pp-cli rubrics update-3` — Update a RubricAssociation
- `canvas-pp-cli rubrics upload` — Creates a rubric using a CSV file
- `canvas-pp-cli rubrics upload-2` — Creates a rubric using a CSV file
- `canvas-pp-cli rubrics upload-status` — Get the status of a rubric import
- `canvas-pp-cli rubrics upload-status-2` — Get the status of a rubric import
- `canvas-pp-cli rubrics upload-template` — Templated file for importing a rubric
- `canvas-pp-cli rubrics used-locations` — Get the courses and assignments for a rubric
- `canvas-pp-cli rubrics used-locations-2` — Get the courses and assignments for a rubric

**search_resource** — [SearchController#recipients](https://github.com/instructure/canvas-lms/blob/master/app/controllers/search_controller.rb)

- `canvas-pp-cli search-resource all-courses` — List all courses
- `canvas-pp-cli search-resource recipients` — Find recipients
- `canvas-pp-cli search-resource recipients-2` — Find recipients

**sections** — API for accessing section information.

- `canvas-pp-cli sections create` — Create course section
- `canvas-pp-cli sections crosslist` — Cross-list a Section
- `canvas-pp-cli sections destroy` — Delete a section
- `canvas-pp-cli sections index` — List course sections
- `canvas-pp-cli sections show` — Get section information
- `canvas-pp-cli sections show-2` — Get section information
- `canvas-pp-cli sections uncrosslist` — De-cross-list a Section
- `canvas-pp-cli sections update` — Edit a section
- `canvas-pp-cli sections users` — List section's users

**services** — [ServicesApiController#show_kaltura_config](https://github.com/instructure/canvas-lms/blob/master/app/controllers/services_api_controller.rb)

- `canvas-pp-cli services show-kaltura-config` — Get Kaltura config
- `canvas-pp-cli services start-kaltura-session` — Start Kaltura session

**shared_brand_configs** — This is how you can share Themes with other people in your account or so you can come back to them later without having to apply them to your account

- `canvas-pp-cli shared-brand-configs create` — Share a BrandConfig (Theme)
- `canvas-pp-cli shared-brand-configs destroy` — Un-share a BrandConfig (Theme)
- `canvas-pp-cli shared-brand-configs update` — Update a shared theme

**sis_import_errors** — {

- `canvas-pp-cli sis-import-errors index` — Get SIS import error list
- `canvas-pp-cli sis-import-errors index-2` — Get SIS import error list

**sis_imports** — API for importing data from Student Information Systems

- `canvas-pp-cli sis-imports abort` — Abort SIS import
- `canvas-pp-cli sis-imports abort-all-pending` — Abort all pending SIS imports
- `canvas-pp-cli sis-imports create` — Import SIS data
- `canvas-pp-cli sis-imports importing` — Get the current importing SIS import
- `canvas-pp-cli sis-imports index` — Get SIS import list
- `canvas-pp-cli sis-imports restore-states` — Restore workflow_states of SIS imported items
- `canvas-pp-cli sis-imports show` — Get SIS import status

**smart_search** — {% hint style="warning" %}

- `canvas-pp-cli smart-search <course_id>` — Search course content

**study_assist** — Student-facing AI-powered study tools (Summarize, Quiz me, Flashcards) backed by Cedar. Scoped to a single course; requires a student enrollment.

- `canvas-pp-cli study-assist <course_id>` — Request a study assist response

**submission_comments** — This API can be used to edit and delete submission comments.

- `canvas-pp-cli submission-comments create-file` — Upload a file
- `canvas-pp-cli submission-comments destroy` — Delete a submission comment
- `canvas-pp-cli submission-comments update` — Edit a submission comment

**submissions** — API for accessing and updating submissions for an assignment. The submission id in these URLs is the id of the student in the course, there is no separate submission id exposed in these APIs.

- `canvas-pp-cli submissions bulk-update` — Grade or comment on multiple submissions
- `canvas-pp-cli submissions bulk-update-2` — Grade or comment on multiple submissions
- `canvas-pp-cli submissions bulk-update-3` — Grade or comment on multiple submissions
- `canvas-pp-cli submissions bulk-update-4` — Grade or comment on multiple submissions
- `canvas-pp-cli submissions create` — Submit an assignment
- `canvas-pp-cli submissions create-2` — Submit an assignment
- `canvas-pp-cli submissions create-file` — Upload a file
- `canvas-pp-cli submissions create-file-2` — Upload a file
- `canvas-pp-cli submissions document-annotations-read-state` — Get document annotations read state
- `canvas-pp-cli submissions document-annotations-read-state-2` — Get document annotations read state
- `canvas-pp-cli submissions for-students` — List submissions for multiple assignments
- `canvas-pp-cli submissions for-students-2` — List submissions for multiple assignments
- `canvas-pp-cli submissions gradeable-students` — List gradeable students
- `canvas-pp-cli submissions index` — List assignment submissions
- `canvas-pp-cli submissions index-2` — List assignment submissions
- `canvas-pp-cli submissions mark-bulk-submissions-as-read` — Mark bulk submissions as read
- `canvas-pp-cli submissions mark-bulk-submissions-as-read-2` — Mark bulk submissions as read
- `canvas-pp-cli submissions mark-document-annotations-read` — Mark document annotations as read
- `canvas-pp-cli submissions mark-document-annotations-read-2` — Mark document annotations as read
- `canvas-pp-cli submissions mark-rubric-assessments-read` — Mark rubric assessments as read
- `canvas-pp-cli submissions mark-rubric-assessments-read-2` — Mark rubric assessments as read
- `canvas-pp-cli submissions mark-rubric-assessments-read-3` — Mark rubric assessments as read
- `canvas-pp-cli submissions mark-rubric-assessments-read-4` — Mark rubric assessments as read
- `canvas-pp-cli submissions mark-submission-item-read` — Mark submission item as read
- `canvas-pp-cli submissions mark-submission-item-read-2` — Mark submission item as read
- `canvas-pp-cli submissions mark-submission-read` — Mark submission as read
- `canvas-pp-cli submissions mark-submission-read-2` — Mark submission as read
- `canvas-pp-cli submissions mark-submission-unread` — Mark submission as unread
- `canvas-pp-cli submissions mark-submission-unread-2` — Mark submission as unread
- `canvas-pp-cli submissions multiple-gradeable-students` — List multiple assignments gradeable students
- `canvas-pp-cli submissions rubric-assessments-read-state` — Get rubric assessments read state
- `canvas-pp-cli submissions rubric-assessments-read-state-2` — Get rubric assessments read state
- `canvas-pp-cli submissions rubric-assessments-read-state-3` — Get rubric assessments read state
- `canvas-pp-cli submissions rubric-assessments-read-state-4` — Get rubric assessments read state
- `canvas-pp-cli submissions show` — Get a single submission
- `canvas-pp-cli submissions show-2` — Get a single submission
- `canvas-pp-cli submissions show-anonymous` — Get a single submission by anonymous id
- `canvas-pp-cli submissions show-anonymous-2` — Get a single submission by anonymous id
- `canvas-pp-cli submissions submission-summary` — Submission Summary
- `canvas-pp-cli submissions submission-summary-2` — Submission Summary
- `canvas-pp-cli submissions submissions-clear-unread` — Clear unread status for all submissions.
- `canvas-pp-cli submissions submissions-clear-unread-2` — Clear unread status for all submissions.
- `canvas-pp-cli submissions update` — Grade or comment on a submission
- `canvas-pp-cli submissions update-2` — Grade or comment on a submission
- `canvas-pp-cli submissions update-anonymous` — Grade or comment on a submission by anonymous id
- `canvas-pp-cli submissions update-anonymous-2` — Grade or comment on a submission by anonymous id

**tabs** — {

- `canvas-pp-cli tabs index` — List available tabs for a course or group
- `canvas-pp-cli tabs index-2` — List available tabs for a course or group
- `canvas-pp-cli tabs index-3` — List available tabs for a course or group
- `canvas-pp-cli tabs index-4` — List available tabs for a course or group
- `canvas-pp-cli tabs update` — Update a tab for a course

**temporary_enrollment_pairings** — // A pairing unique to that enrollment period given to a recipient of that

- `canvas-pp-cli temporary-enrollment-pairings create` — Create Temporary Enrollment Pairing
- `canvas-pp-cli temporary-enrollment-pairings destroy` — Delete Temporary Enrollment Pairing
- `canvas-pp-cli temporary-enrollment-pairings index` — List temporary enrollment pairings
- `canvas-pp-cli temporary-enrollment-pairings new` — New TemporaryEnrollmentPairing
- `canvas-pp-cli temporary-enrollment-pairings show` — Get a single temporary enrollment pairing

**user_observees** — API for managing linked observers and observees

- `canvas-pp-cli user-observees create` — Add an observee with credentials
- `canvas-pp-cli user-observees create-2` — Create observer pairing code
- `canvas-pp-cli user-observees destroy` — Remove an observee
- `canvas-pp-cli user-observees index` — List linked observees
- `canvas-pp-cli user-observees observers` — List linked observers
- `canvas-pp-cli user-observees show` — Show an observee
- `canvas-pp-cli user-observees show-observer` — Show an observer
- `canvas-pp-cli user-observees update` — Add an observee

**users** — API for accessing information on the current and other users.

- `canvas-pp-cli users activity-stream` — List the activity stream
- `canvas-pp-cli users activity-stream-2` — List the activity stream
- `canvas-pp-cli users activity-stream-summary` — Activity stream summary
- `canvas-pp-cli users api-index` — List users in account
- `canvas-pp-cli users api-show` — Show user details
- `canvas-pp-cli users batch-query` — BETA - Initiate batch page views query
- `canvas-pp-cli users batch-query-results` — BETA - Get batch query results
- `canvas-pp-cli users clear` — Clear course nicknames
- `canvas-pp-cli users create` — Create a user
- `canvas-pp-cli users create-file` — Upload a file
- `canvas-pp-cli users create-self-registered-user` — [DEPRECATED] Self register a user
- `canvas-pp-cli users delete` — Remove course nickname
- `canvas-pp-cli users delete-data` — Delete custom data
- `canvas-pp-cli users expire-mobile-sessions` — Log users out of all mobile apps
- `canvas-pp-cli users expire-mobile-sessions-2` — Log users out of all mobile apps
- `canvas-pp-cli users get-custom-color` — Get custom color
- `canvas-pp-cli users get-custom-colors` — Get custom colors
- `canvas-pp-cli users get-dashboard-positions` — Get dashboard positions
- `canvas-pp-cli users get-data` — Load custom data
- `canvas-pp-cli users ignore-all-stream-items` — Hide all stream items
- `canvas-pp-cli users ignore-stream-item` — Hide a stream item
- `canvas-pp-cli users index` — List user page views
- `canvas-pp-cli users index-2` — List course nicknames
- `canvas-pp-cli users merge-into` — Merge user into another user
- `canvas-pp-cli users merge-into-2` — Merge user into another user
- `canvas-pp-cli users missing-submissions` — List Missing Submissions
- `canvas-pp-cli users pandata-events-token` — Get a Pandata Events jwt token and its expiration date
- `canvas-pp-cli users poll-batch-query` — BETA - Poll batch query status
- `canvas-pp-cli users poll-query` — BETA - Poll query status
- `canvas-pp-cli users profile-pics` — List avatar options
- `canvas-pp-cli users query` — BETA - Initiate page views query
- `canvas-pp-cli users query-results` — BETA - Get query results
- `canvas-pp-cli users set-custom-color` — Update custom color
- `canvas-pp-cli users set-dashboard-positions` — Update dashboard positions
- `canvas-pp-cli users set-data` — Store custom data
- `canvas-pp-cli users set-files-ui-version-preference` — Update files UI version preference
- `canvas-pp-cli users set-text-editor-preference` — Update text editor preference
- `canvas-pp-cli users settings` — Update user settings.
- `canvas-pp-cli users settings-2` — Update user settings.
- `canvas-pp-cli users settings-3` — Get user profile
- `canvas-pp-cli users show` — Get course nickname
- `canvas-pp-cli users split` — Split merged users into separate users
- `canvas-pp-cli users terminate-sessions` — Terminate all user sessions
- `canvas-pp-cli users todo-item-count` — List counts for todo items
- `canvas-pp-cli users todo-items` — List the TODO items
- `canvas-pp-cli users upcoming-events` — List upcoming assignments, calendar events
- `canvas-pp-cli users update` — Edit a user
- `canvas-pp-cli users update-2` — Set course nickname
- `canvas-pp-cli users user-graded-submissions` — Get a users most recently graded submissions

**what_if_grades** — {

- `canvas-pp-cli what-if-grades reset-for-student-course` — Reset the what-if scores for the current user for an entire course and recalculate grades
- `canvas-pp-cli what-if-grades update` — Update a submission's what-if score and calculate grades


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
canvas-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query.

## Recipes


### Course roster as JSON for an agent

```bash
canvas-pp-cli roster 12345 --agent --select students.name,students.section,students.current_score
```

Narrows the joined roster to just the fields an agent needs, keeping token use low.

### Cross-course grading queue

```bash
canvas-pp-cli to-grade --all-my-courses --sort oldest --agent
```

One oldest-first list of everything you still owe a grade on, across every course you teach.

### Find a student across courses

```bash
canvas-pp-cli search "Jordan" --type user --agent
```

Offline full-text search over the synced local store finds people without an API round-trip.

### Privacy-safe at-risk export

```bash
canvas-pp-cli at-risk --course 12345 --since 7d --anonymize --agent
```

Hashes names and drops SIS/login IDs so the list is safe to hand to an AI assistant.

### Enroll a student via the typed endpoint

```bash
canvas-pp-cli enrollments create 12345 --enrollment-user-id 987 --enrollment-type StudentEnrollment --dry-run
```

Preview the request first; drop --dry-run to enroll. Every one of the 1,042 endpoints is reachable this way.

## Auth Setup

Canvas uses a per-institution base URL and a Bearer access token. Set CANVAS_BASE_URL to your instance host (e.g. https://canvas.project-remedy.com) and CANVAS_API_TOKEN to a token from Account > Settings > New Access Token. Tokens are user-scoped, so the CLI can only do what your Canvas account can.

Run `canvas-pp-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  canvas-pp-cli account-domain-lookups --agent --select id,name,status
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Explicit retries** — use `--idempotent` only when an already-existing create should count as success, and `--ignore-missing` only when a missing delete target should count as success

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal AND no machine-format flag (`--json`, `--csv`, `--compact`, `--quiet`, `--plain`, `--select`) is set — piped/agent consumers and explicit-format runs get pure JSON on stdout.

## Paths and state

Agents should treat the CLI's path resolver as part of the runtime contract:

- Use `--home <dir>` for one invocation, or set `CANVAS_HOME=<dir>` to relocate all four path kinds under one root.
- Use per-kind env vars only when a specific kind must diverge: `CANVAS_CONFIG_DIR`, `CANVAS_DATA_DIR`, `CANVAS_STATE_DIR`, `CANVAS_CACHE_DIR`.
- Resolution order is per-kind env var, `--home`, `CANVAS_HOME`, XDG (`XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`), then platform defaults.
- `config` contains settings like `config.toml` and profiles. `data` contains `credentials.toml`, `data.db`, cookies, and auth sidecars. `state` contains persisted queries, jobs, and `teach.log`. `cache` contains regenerable HTTP/cache files.
- Stored secrets live in `credentials.toml` under the data dir. Existing legacy `config.toml` secrets are read for compatibility and leave `config.toml` on the first auth write.
- Run `canvas-pp-cli doctor --fail-on warn` to surface path and credential-location warnings. `agent-context` exposes a schema v4 `paths` block for agents that need the resolved dirs.
- For MCP, pass relocation through the MCP host config. The MCP binary does not inherit CLI flags:

  ```json
  {
    "mcpServers": {
      "canvas": {
        "command": "canvas-pp-mcp",
        "env": {
          "CANVAS_HOME": "/srv/canvas"
        }
      }
    }
  }
  ```

Fleet precedence: an inherited per-kind env var overrides an explicit `--home` for that kind. Use `CANVAS_HOME` or per-kind vars as durable fleet levers, and use `--home` only for a single invocation. Relocation is not reversible by unsetting env vars; move files manually before clearing `CANVAS_HOME`, or `doctor` will not find credentials left under the former root.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
canvas-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
canvas-pp-cli feedback --stdin < notes.txt
canvas-pp-cli feedback list --json --limit 10
```

Entries are stored locally as `feedback.jsonl` under the resolved data dir. They are never POSTed unless `CANVAS_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `CANVAS_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
canvas-pp-cli profile save briefing --json
canvas-pp-cli --profile briefing account-domain-lookups
canvas-pp-cli profile list --json
canvas-pp-cli profile show briefing
canvas-pp-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `canvas-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

**Only for hosts that cannot run shell commands** — Claude Desktop, ChatGPT Desktop. If you can run `canvas-pp-cli` in a terminal, do that instead (see Direct Use below); `canvas-pp-mcp` just shells out to the same CLI binary, so it adds nothing for a shell-capable agent.

1. Install both binaries, into the same directory. `canvas-pp-mcp` finds the CLI by looking in its own directory first, then `CANVAS_CLI_PATH`, then `$PATH`; installing only the server gives `companion CLI binary not found`:
   ```bash
   go install github.com/johnnyrobot/canvas-pp-cli/cmd/canvas-pp-cli@latest
   go install github.com/johnnyrobot/canvas-pp-cli/cmd/canvas-pp-mcp@latest
   ```
2. Register with the host, passing both env vars. `CANVAS_BASE_URL` is required — Canvas has no single host. For Claude Desktop, add to `claude_desktop_config.json`:
   ```json
   {
     "mcpServers": {
       "canvas": {
         "command": "canvas-pp-mcp",
         "env": {
           "CANVAS_BASE_URL": "https://your-school.instructure.com",
           "CANVAS_API_TOKEN": "your-token"
         }
       }
     }
   }
   ```
   For ChatGPT Desktop, run `canvas-pp-mcp -transport http -addr :7777` and add `http://localhost:7777` as a connector.
3. The server exposes `canvas_search` to find an endpoint and `canvas_execute` to call it.

## Direct Use

1. Check if installed: `which canvas-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   canvas-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `canvas-pp-cli <command> --help`.
