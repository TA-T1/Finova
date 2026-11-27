import sys, pathlib
sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / "features"))
import pytest
from feature_engineering import build_panel
import lookahead_check as L


@pytest.fixture(scope="module")
def panel():
    return build_panel()


def test_cut_the_future(panel):
    assert L.run_all(panel)["cut"] == {}


def test_perturb_the_future(panel):
    assert L.run_all(panel)["perturb"] == {}


def test_leaky_feature_is_detected(panel):
    assert L.run_all(panel)["leaky_detected"]
