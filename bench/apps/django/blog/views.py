import json

from django.conf import settings
from django.views.decorators.http import require_safe
from inertia import render

# The page every app in the benchmark serves, by its path from the app, read
# once per process, not per request: as Django imports this module with the
# URLconf, which it does at a worker's first request.
PAGE = json.loads((settings.BASE_DIR / '../../page/page.json').read_bytes())


# GET and HEAD, as the other frameworks' GET routes take both.
@require_safe
def show(request, id):
    return render(request, 'Posts/Show', props={
        # A copy with the route's ID, which keeps 'id' the post's first key.
        'post': {**PAGE['post'], 'id': id},
        'comments': PAGE['comments'],
    })
