// Odoo feature detection must see service workers as unavailable, not broken.
delete Navigator.prototype.serviceWorker;
