-- An exact-title pass names one show. title leads, with NOCASE so the pass
-- and the listing share a key when only the letter case differs, and starts_at
-- is the window's upper bound. ends_at stays off this index: a window index
-- cannot also seek one title, and the channel index must keep its own plan.
CREATE INDEX airings_title_start ON airings(title COLLATE NOCASE, starts_at);
