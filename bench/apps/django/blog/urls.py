"""
URL configuration for blog project: the one page, and nothing else, so the
admin's route that startproject adds is gone.
"""
from django.urls import path

from . import views

urlpatterns = [
    path('posts/<int:id>', views.show),
]
