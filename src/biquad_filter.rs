/* SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Copyright (C) 2025 The OpenPSG Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

use core::marker::PhantomData;
use num_traits::{FromPrimitive, ToPrimitive};

/// A simple biquad filter implementation
pub struct BiquadFilter<T>
where
    T: FromPrimitive + ToPrimitive,
{
    numerator: [f32; 3],   // Numerator coefficients
    denominator: [f32; 3], // Denominator coefficients
    index: usize,          // Index of the current sample
    y1: f32,
    y2: f32,
    x1: f32,
    x2: f32,
    _sample_type: PhantomData<T>,
}

impl<T> BiquadFilter<T>
where
    T: FromPrimitive + ToPrimitive,
{
    /// Create a new biquad filter with the given numerator and denominator coefficients.
    pub fn new(numerator: [f32; 3], denominator: [f32; 3]) -> Self {
        assert!(
            denominator[0] == 1.0,
            "denominator[0] should be 1 for proper scaling"
        );

        BiquadFilter {
            numerator,
            denominator,
            index: 0,
            y1: 0.0,
            y2: 0.0,
            x1: 0.0,
            x2: 0.0,
            _sample_type: PhantomData,
        }
    }

    // Apply the filter to an array of samples in place.
    pub fn apply(&mut self, samples: &mut [T]) {
        for sample in samples.iter_mut() {
            let input = sample.to_f32().unwrap_or(0.0);

            // Prevent ringing if the filter is reset.
            if self.index == 0 {
                self.x1 = input;
                self.x2 = input;
                self.y1 = input;
                self.y2 = input;
            }
            self.index += 1;

            let output = self.numerator[0] * input
                + self.numerator[1] * self.x1
                + self.numerator[2] * self.x2
                - self.denominator[1] * self.y1
                - self.denominator[2] * self.y2;

            self.x2 = self.x1;
            self.x1 = input;
            self.y2 = self.y1;
            self.y1 = output;

            *sample = T::from_f32(output).unwrap_or_else(|| T::from_f32(0.0).unwrap());
        }
    }
}
