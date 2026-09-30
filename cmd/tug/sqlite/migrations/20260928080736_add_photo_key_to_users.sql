-- Where each user's photo is kept on the app's disk: photos.go.
ALTER TABLE users ADD COLUMN photo_key TEXT;
