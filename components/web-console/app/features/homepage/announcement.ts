import { defineMessages, type MessageDescriptor } from "react-intl";

export interface HomepageAnnouncement {
  readonly title: MessageDescriptor;
  readonly body: MessageDescriptor;
}

const announcementMessages = defineMessages({
  title: {
    id: "app.homepage.announcement.example.title",
    defaultMessage: "Scheduled downtime",
    description: "Title of the example homepage announcement.",
  },
  body: {
    id: "app.homepage.announcement.example.body",
    defaultMessage:
      "HyperShell will be unavailable on Oct 24 from 02:00 to 03:00 CT while we upgrade the platform.",
    description: "Body of the example homepage announcement.",
  },
});

/**
 * Controls the announcement alert at the top of the homepage.
 *
 * To publish an announcement, assign `homepageAnnouncementExample` here and
 * edit its title and body in `announcementMessages`. To hide the announcement,
 * restore this value to `null`.
 */
export const homepageAnnouncement: HomepageAnnouncement | null = null;

export const homepageAnnouncementExample: HomepageAnnouncement = {
  title: announcementMessages.title,
  body: announcementMessages.body,
};
